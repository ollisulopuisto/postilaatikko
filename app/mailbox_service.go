package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hkdb/aerion/internal/automation"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/keyring"
	"github.com/hkdb/aerion/internal/views"
)

const (
	desktopMailboxClientName = "Postilaatikko Desktop Local"
	desktopMailboxSecretName = "mailbox_service_client"
	mailboxServiceEndpoint   = "http://127.0.0.1:8029"
)

var errDesktopMailboxAuthorizationRequired = errors.New("desktop mailbox service authorization required")

type desktopMailboxCredential struct {
	ClientID string `json:"clientId"`
	Token    string `json:"token"`
}

// MailboxServiceStatus describes the local service connection without exposing
// the bearer token or raw server responses.
type MailboxServiceStatus struct {
	Connected    bool     `json:"connected"`
	APIVersion   string   `json:"apiVersion,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	ErrorCode    string   `json:"errorCode,omitempty"`
	Message      string   `json:"message,omitempty"`
}

type mailboxServiceConnector struct {
	mu         sync.Mutex
	db         *database.DB
	keyring    *keyring.Keyring
	credential *desktopMailboxCredential
	client     *automationclient.Client
	searchMu   sync.Mutex
	searches   map[string]activeMailboxSearch
}

type activeMailboxSearch struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func newMailboxServiceConnector(db *database.DB) *mailboxServiceConnector {
	return &mailboxServiceConnector{db: db, keyring: keyring.New()}
}

func (c *mailboxServiceConnector) getClient() (*automationclient.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	if c.db == nil || c.keyring == nil {
		return nil, errors.New("mailbox service connection is not initialized")
	}

	tokenStore := automation.NewSQLiteTokenStore(c.db)
	secret, err := c.keyring.GetSecret(desktopMailboxSecretName)
	if err == nil {
		credential := desktopMailboxCredential{}
		if err := json.Unmarshal([]byte(secret), &credential); err != nil || credential.ClientID == "" || credential.Token == "" {
			return nil, errDesktopMailboxAuthorizationRequired
		}
		grant, grantErr := tokenStore.GetGrant(credential.Token)
		if errors.Is(grantErr, automation.ErrClientNotFound) || errors.Is(grantErr, automation.ErrTokenRevoked) {
			return nil, errDesktopMailboxAuthorizationRequired
		}
		if grantErr != nil {
			return nil, fmt.Errorf("validate desktop mailbox credential: %w", grantErr)
		}
		if grant.ClientID != credential.ClientID || !hasCapability(grant.Capabilities, v1.CapabilityMailRead) || !hasCapability(grant.Capabilities, v1.CapabilityMailBodyFetch) {
			return nil, errDesktopMailboxAuthorizationRequired
		}
		c.credential = &credential
		c.client = newMailboxAPIClient(credential.Token)
		return c.client, nil
	} else if !errors.Is(err, keyring.ErrCredentialNotFound) {
		return nil, err
	}

	clients, err := tokenStore.ListClients()
	if err != nil {
		return nil, fmt.Errorf("list desktop mailbox credentials: %w", err)
	}
	for _, existing := range clients {
		if existing != nil && existing.Name == desktopMailboxClientName {
			return nil, errDesktopMailboxAuthorizationRequired
		}
	}

	grant, err := tokenStore.RegisterClient(desktopMailboxClientName, []v1.Capability{v1.CapabilityMailRead, v1.CapabilityMailBodyFetch})
	if err != nil {
		return nil, fmt.Errorf("provision desktop mailbox credential: %w", err)
	}
	encoded, err := json.Marshal(desktopMailboxCredential{ClientID: grant.ClientID, Token: grant.Token})
	if err != nil {
		_ = tokenStore.RevokeClient(grant.ClientID)
		return nil, fmt.Errorf("encode desktop mailbox credential: %w", err)
	}
	if err := c.keyring.SetSecret(desktopMailboxSecretName, string(encoded)); err != nil {
		_ = tokenStore.RevokeClient(grant.ClientID)
		return nil, err
	}
	c.credential = &desktopMailboxCredential{ClientID: grant.ClientID, Token: grant.Token}
	c.client = newMailboxAPIClient(grant.Token)
	return c.client, nil
}

func newMailboxAPIClient(token string) *automationclient.Client {
	return automationclient.New(automationclient.Config{
		Endpoint: mailboxServiceEndpoint,
		Token:    token,
		Timeout:  2 * time.Second,
	})
}

func hasCapability(capabilities []v1.Capability, required v1.Capability) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func (c *mailboxServiceConnector) status(ctx context.Context) MailboxServiceStatus {
	client, err := c.getClient()
	if err != nil {
		if errors.Is(err, errDesktopMailboxAuthorizationRequired) {
			return MailboxServiceStatus{ErrorCode: "authorization_required", Message: "Desktop access to the local mailbox service needs repair."}
		}
		return MailboxServiceStatus{ErrorCode: "credential_unavailable", Message: "Desktop service credentials are unavailable."}
	}
	c.mu.Lock()
	credential := c.credential
	c.mu.Unlock()
	if credential == nil {
		return MailboxServiceStatus{ErrorCode: "credential_unavailable", Message: "Desktop service credentials are unavailable."}
	}
	grant, err := automation.NewSQLiteTokenStore(c.db).GetGrant(credential.Token)
	if errors.Is(err, automation.ErrClientNotFound) || errors.Is(err, automation.ErrTokenRevoked) || (err == nil && grant.ClientID != credential.ClientID) {
		return MailboxServiceStatus{ErrorCode: "authorization_required", Message: "Desktop access to the local mailbox service needs repair."}
	}
	if err != nil {
		return MailboxServiceStatus{ErrorCode: "credential_unavailable", Message: "Desktop service credentials are unavailable."}
	}
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	capabilities, err := client.GetCapabilities(statusCtx)
	if err != nil {
		return MailboxServiceStatus{ErrorCode: "service_unavailable", Message: "The local mailbox service is not reachable."}
	}
	if capabilities == nil || capabilities.Version != v1.Version || !hasCapability(capabilities.Capabilities, v1.CapabilityMailRead) || !hasCapability(capabilities.Capabilities, v1.CapabilityMailBodyFetch) {
		return MailboxServiceStatus{ErrorCode: "capability_mismatch", Message: "The local mailbox service does not provide the required read and body-fetch capabilities."}
	}
	granted := make([]string, 0, len(capabilities.Capabilities))
	for _, capability := range capabilities.Capabilities {
		granted = append(granted, string(capability))
	}
	return MailboxServiceStatus{
		Connected:    true,
		APIVersion:   capabilities.Version,
		Capabilities: granted,
	}
}

func (c *mailboxServiceConnector) fetchMessageBody(ctx context.Context, accountID, messageID string) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	_, err = client.ReadBody(ctx, v1.BodyReadRequest{
		Ref:  v1.MessageRef{AccountID: accountID, MessageID: messageID},
		Mode: v1.FetchExplicit,
		// The desktop reader needs the original display body, not only the
		// derived reply excerpt returned by default.
		IncludeOriginal: true,
	})
	return err
}

func (c *mailboxServiceConnector) listConversations(ctx context.Context, req v1.ConversationListRequest) (*v1.ConversationListPage, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.ListConversations(ctx, req)
}

func (c *mailboxServiceConnector) search(ctx context.Context, sessionID string, req v1.SearchRequest) (*views.ThreadPage, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	searchCtx, cancel := context.WithCancel(ctx)
	if sessionID != "" {
		c.searchMu.Lock()
		if c.searches == nil {
			c.searches = map[string]activeMailboxSearch{}
		}
		if previous, ok := c.searches[sessionID]; ok {
			previous.cancel()
		}
		c.searches[sessionID] = activeMailboxSearch{ctx: searchCtx, cancel: cancel}
		c.searchMu.Unlock()
	}
	defer func() {
		cancel()
		if sessionID == "" {
			return
		}
		c.searchMu.Lock()
		defer c.searchMu.Unlock()
		if current, ok := c.searches[sessionID]; ok && current.ctx == searchCtx {
			delete(c.searches, sessionID)
		}
	}()
	return client.Search(searchCtx, req)
}

func (c *mailboxServiceConnector) readThread(ctx context.Context, ref views.ThreadRef, scope views.Scope) (*views.ThreadView, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.ReadThread(ctx, v1.ThreadReadRequest{Ref: ref, Scope: scope})
}

func (c *mailboxServiceConnector) cancelSearch(sessionID string) {
	if sessionID == "" {
		return
	}
	c.searchMu.Lock()
	defer c.searchMu.Unlock()
	if active, ok := c.searches[sessionID]; ok {
		active.cancel()
		delete(c.searches, sessionID)
	}
}

// GetMailboxServiceStatus checks the authenticated localhost service without
// allowing a missing service to block application startup.
func (a *App) GetMailboxServiceStatus() MailboxServiceStatus {
	if a.mailboxService == nil {
		return MailboxServiceStatus{ErrorCode: "service_unavailable", Message: "The local mailbox service is not reachable."}
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.mailboxService.status(ctx)
}
