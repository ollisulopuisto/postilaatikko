package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/automation"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/keyring"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/operations"
	"github.com/hkdb/aerion/internal/replylater"
	"github.com/hkdb/aerion/internal/servicebootstrap"
	"github.com/hkdb/aerion/internal/triage"
	"github.com/hkdb/aerion/internal/views"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

const (
	desktopMailboxClientName = "Postilaatikko Desktop Local"
	desktopMailboxSecretName = "mailbox_service_client"
	mailboxServiceEndpoint   = "http://127.0.0.1:8029"
)

var (
	errDesktopMailboxAuthorizationRequired      = errors.New("desktop mailbox service authorization required")
	errDesktopMailboxWriteAuthorizationRequired = errors.New("desktop mailbox service needs explicit mail-write authorization")
)

type desktopMailboxCredential struct {
	ClientID string `json:"clientId"`
	Token    string `json:"token"`
}

// MailboxServiceStatus describes the local service connection without exposing
// the bearer token or raw server responses.
type MailboxServiceStatus struct {
	Connected       bool     `json:"connected"`
	WriteAuthorized bool     `json:"writeAuthorized"`
	APIVersion      string   `json:"apiVersion,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	ErrorCode       string   `json:"errorCode,omitempty"`
	Message         string   `json:"message,omitempty"`
}

// MailboxServiceLifecycleStatus describes the local OS service controls.
type MailboxServiceLifecycleStatus struct {
	Supported       bool `json:"supported"`
	HelperAvailable bool `json:"helperAvailable"`
	Installed       bool `json:"installed"`
	Running         bool `json:"running"`
}

type helperAgentStatus struct {
	Supported bool `json:"supported"`
	Installed bool `json:"installed"`
	Running   bool `json:"running"`
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
		Connected:       true,
		WriteAuthorized: hasCapability(capabilities.Capabilities, v1.CapabilityMailWrite),
		APIVersion:      capabilities.Version,
		Capabilities:    granted,
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

func (c *mailboxServiceConnector) mutateAction(ctx context.Context, req v1.ActionRequest) (*v1.ActionResponse, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.MutateAction(ctx, req, uuid.NewString())
}

func (c *mailboxServiceConnector) virtualFolders(ctx context.Context) ([]*virtualfolder.VirtualFolder, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.VirtualFolders(ctx)
}

func (c *mailboxServiceConnector) mutateVirtualFolder(ctx context.Context, req v1.VirtualFolderMutationRequest) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	return client.MutateVirtualFolder(ctx, req)
}

func (c *mailboxServiceConnector) filterRules(ctx context.Context) ([]*mailfilter.Rule, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetFilterRules(ctx)
}

func (c *mailboxServiceConnector) mutateFilterRule(ctx context.Context, req v1.FilterRuleMutationRequest) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	return client.MutateFilterRule(ctx, req)
}

func (c *mailboxServiceConnector) replyLaterQueue(ctx context.Context) ([]replylater.Ref, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetReplyLaterQueue(ctx)
}

func (c *mailboxServiceConnector) setReplyLaterMembership(ctx context.Context, ref replylater.Ref, inQueue bool) (bool, error) {
	client, err := c.getClient()
	if err != nil {
		return false, err
	}
	return client.SetReplyLaterMembership(ctx, ref, inQueue)
}

func (c *mailboxServiceConnector) readLaterQueue(ctx context.Context) ([]triage.Ref, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetReadLaterQueue(ctx)
}

func (c *mailboxServiceConnector) setReadLater(ctx context.Context, ref triage.Ref, inQueue bool) (bool, error) {
	client, err := c.getClient()
	if err != nil {
		return false, err
	}
	return client.SetReadLater(ctx, ref, inQueue)
}

func (c *mailboxServiceConnector) snoozedConversations(ctx context.Context) ([]triage.Ref, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetSnoozedConversations(ctx)
}

func (c *mailboxServiceConnector) snoozedCount(ctx context.Context) (int, error) {
	client, err := c.getClient()
	if err != nil {
		return 0, err
	}
	return client.GetSnoozedCount(ctx)
}

func (c *mailboxServiceConnector) snooze(ctx context.Context, ref triage.Ref, wakeAt string) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	return client.SnoozeConversation(ctx, ref, wakeAt)
}

func (c *mailboxServiceConnector) unsnooze(ctx context.Context, ref triage.Ref) (bool, error) {
	client, err := c.getClient()
	if err != nil {
		return false, err
	}
	return client.UnsnoozeConversation(ctx, ref)
}

func (c *mailboxServiceConnector) authorizeWrites(ctx context.Context) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	capabilities, err := client.GetCapabilities(ctx)
	if err != nil {
		return fmt.Errorf("check mailbox service capabilities: %w", err)
	}
	if capabilities == nil || capabilities.Version != v1.Version || !hasCapability(capabilities.Capabilities, v1.CapabilityMailRead) || !hasCapability(capabilities.Capabilities, v1.CapabilityMailBodyFetch) {
		return errors.New("mailbox service read capabilities are unavailable")
	}
	if hasCapability(capabilities.Capabilities, v1.CapabilityMailWrite) {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.credential == nil || c.db == nil || c.keyring == nil {
		return errDesktopMailboxAuthorizationRequired
	}
	oldCredential := *c.credential
	tokenStore := automation.NewSQLiteTokenStore(c.db)
	oldGrant, err := tokenStore.GetGrant(oldCredential.Token)
	if err != nil || oldGrant.ClientID != oldCredential.ClientID {
		return errDesktopMailboxAuthorizationRequired
	}
	grant, err := tokenStore.RegisterClient(desktopMailboxClientName, []v1.Capability{
		v1.CapabilityMailRead,
		v1.CapabilityMailBodyFetch,
		v1.CapabilityMailWrite,
	})
	if err != nil {
		return fmt.Errorf("create write-enabled desktop grant: %w", err)
	}
	encoded, err := json.Marshal(desktopMailboxCredential{ClientID: grant.ClientID, Token: grant.Token})
	if err != nil {
		_ = tokenStore.RevokeClient(grant.ClientID)
		return fmt.Errorf("encode write-enabled desktop grant: %w", err)
	}
	if err := c.keyring.SetSecret(desktopMailboxSecretName, string(encoded)); err != nil {
		_ = tokenStore.RevokeClient(grant.ClientID)
		return fmt.Errorf("save write-enabled desktop grant: %w", err)
	}
	if err := tokenStore.RevokeClient(oldCredential.ClientID); err != nil {
		oldEncoded, marshalErr := json.Marshal(oldCredential)
		if marshalErr == nil {
			if restoreErr := c.keyring.SetSecret(desktopMailboxSecretName, string(oldEncoded)); restoreErr == nil {
				_ = tokenStore.RevokeClient(grant.ClientID)
				return fmt.Errorf("revoke previous desktop grant: %w", err)
			}
		}
		// The replacement is already stored in the keyring. Keep the in-memory
		// connector aligned with it if rollback cannot restore the old secret.
		c.credential = &desktopMailboxCredential{ClientID: grant.ClientID, Token: grant.Token}
		c.client = newMailboxAPIClient(grant.Token)
		return fmt.Errorf("write access is enabled, but the previous read-only grant could not be revoked: %w", err)
	}
	c.credential = &desktopMailboxCredential{ClientID: grant.ClientID, Token: grant.Token}
	c.client = newMailboxAPIClient(grant.Token)
	return nil
}

func (a *App) mutateFlagThroughMailboxService(ctx context.Context, messages []*message.Message, action v1.ActionKind) (bool, error) {
	_, handled, err := a.mutateMessagesThroughMailboxService(ctx, messages, v1.ActionRequest{Action: action})
	return handled, err
}

func (a *App) mutateMessagesThroughMailboxService(ctx context.Context, messages []*message.Message, req v1.ActionRequest) (*v1.ActionResponse, bool, error) {
	if a.mailboxService == nil {
		return nil, false, nil
	}
	status := a.mailboxService.status(ctx)
	if !status.Connected {
		if status.ErrorCode == "service_unavailable" {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("local mailbox service is unavailable for %s: %s", req.Action, status.Message)
	}
	if !status.WriteAuthorized {
		return nil, true, errDesktopMailboxWriteAuthorizationRequired
	}
	refs := make([]operations.MessageRef, 0, len(messages))
	for _, item := range messages {
		if item == nil {
			continue
		}
		if item.ID == "" || item.AccountID == "" {
			return nil, true, errors.New("mailbox service action requires account-qualified message identities")
		}
		refs = append(refs, operations.MessageRef{AccountID: item.AccountID, MessageID: item.ID})
	}
	if len(refs) == 0 {
		return &v1.ActionResponse{}, true, nil
	}
	req.Target = operations.Target{Kind: operations.TargetMatchedMessages, Messages: refs}
	response, err := a.mailboxService.mutateAction(ctx, req)
	if err != nil {
		return nil, true, fmt.Errorf("run %s through local mailbox service: %w", req.Action, err)
	}
	return response, true, nil
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

// GetMailboxServiceLifecycleStatus reports whether the bundled helper's
// per-user service can be managed and whether its LaunchAgent is installed.
func (a *App) GetMailboxServiceLifecycleStatus() (MailboxServiceLifecycleStatus, error) {
	if runtime.GOOS != "darwin" {
		return MailboxServiceLifecycleStatus{}, nil
	}
	status := MailboxServiceLifecycleStatus{Supported: true}
	if _, err := a.mailboxHelperPath(); err != nil {
		return status, err
	}
	status.HelperAvailable = true
	output, err := a.runMailboxHelperCommand("agent-status")
	if err != nil {
		return status, err
	}
	var agentStatus helperAgentStatus
	if err := json.Unmarshal(output, &agentStatus); err != nil {
		return status, fmt.Errorf("decode helper service status: %w", err)
	}
	status.Supported = agentStatus.Supported
	status.Installed = agentStatus.Installed
	status.Running = agentStatus.Running
	return status, nil
}

// InstallMailboxService installs and starts the bundled local helper agent.
func (a *App) InstallMailboxService() (MailboxServiceLifecycleStatus, error) {
	if runtime.GOOS != "darwin" {
		return MailboxServiceLifecycleStatus{}, errors.New("local helper service installation is supported on macOS only")
	}
	if a.paths == nil || a.paths.Data == "" {
		return MailboxServiceLifecycleStatus{}, errors.New("application data directory is unavailable")
	}
	queuePath := filepath.Join(a.paths.Data, "send-later-helper")
	if _, err := a.runMailboxHelperCommand("install-agent", "--identity", "studio", "--data", a.paths.Data, "--queue", queuePath); err != nil {
		return MailboxServiceLifecycleStatus{}, err
	}
	return a.GetMailboxServiceLifecycleStatus()
}

// StartMailboxService starts an installed helper agent without changing its configuration.
func (a *App) StartMailboxService() (MailboxServiceLifecycleStatus, error) {
	if _, err := a.runMailboxHelperCommand("start-agent"); err != nil {
		return MailboxServiceLifecycleStatus{}, err
	}
	return a.GetMailboxServiceLifecycleStatus()
}

// StopMailboxService stops the helper agent while retaining its LaunchAgent configuration.
func (a *App) StopMailboxService() (MailboxServiceLifecycleStatus, error) {
	if _, err := a.runMailboxHelperCommand("stop-agent"); err != nil {
		return MailboxServiceLifecycleStatus{}, err
	}
	return a.GetMailboxServiceLifecycleStatus()
}

func (a *App) mailboxHelperPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate Postilaatikko app executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve Postilaatikko app executable: %w", err)
	}
	helperName := "postilaatikko-helper"
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	helperPath := filepath.Join(filepath.Dir(executable), helperName)
	info, err := os.Stat(helperPath)
	if err != nil {
		return "", fmt.Errorf("locate bundled mailbox helper: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return "", errors.New("bundled mailbox helper is not an executable file")
	}
	return helperPath, nil
}

func (a *App) bootstrapMailboxDatabase() error {
	if a.paths == nil || a.paths.Data == "" {
		return errors.New("application data directory is unavailable for mailbox service bootstrap")
	}
	output, err := a.runMailboxHelperCommand("bootstrap", "--data", a.paths.Data)
	if err != nil {
		return fmt.Errorf("prepare mailbox service database: %w", err)
	}
	var status servicebootstrap.Status
	if err := json.Unmarshal(output, &status); err != nil {
		return fmt.Errorf("decode mailbox service bootstrap response: %w", err)
	}
	if status.ProtocolVersion != servicebootstrap.ProtocolVersion || status.APIVersion != v1.Version {
		return fmt.Errorf("mailbox helper bootstrap/API version %d/%s is incompatible with desktop %d/%s", status.ProtocolVersion, status.APIVersion, servicebootstrap.ProtocolVersion, v1.Version)
	}
	if status.MaxSchemaVersion != database.MaxSchemaVersion() || status.SchemaVersion != status.MaxSchemaVersion {
		return fmt.Errorf("mailbox helper reported incompatible database schema %d/%d; desktop supports %d", status.SchemaVersion, status.MaxSchemaVersion, database.MaxSchemaVersion())
	}
	if err := a.verifyRunningMailboxService(); err != nil {
		return err
	}
	return nil
}

func (a *App) verifyRunningMailboxService() error {
	connection, err := net.DialTimeout("tcp", "127.0.0.1:8028", 300*time.Millisecond)
	if err != nil {
		var operationError *net.OpError
		if errors.As(err, &operationError) && errors.Is(operationError.Err, syscall.ECONNREFUSED) {
			return nil
		}
		return fmt.Errorf("check local mailbox helper compatibility: %w", err)
	}
	_ = connection.Close()
	client := &http.Client{Timeout: 300 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Get("http://127.0.0.1:8028/healthz")
	if err != nil {
		return fmt.Errorf("read local mailbox helper compatibility: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("running local mailbox service returned health status %d", response.StatusCode)
	}
	var status struct {
		Service string `json:"service"`
		servicebootstrap.Status
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&status); err != nil {
		return fmt.Errorf("decode running mailbox service status: %w", err)
	}
	if status.Service != "mail-service" && status.Service != "send-later" {
		return errors.New("another service is listening on the local mailbox helper port")
	}
	if status.ProtocolVersion != servicebootstrap.ProtocolVersion ||
		status.APIVersion != v1.Version ||
		status.SchemaVersion != database.MaxSchemaVersion() ||
		status.MaxSchemaVersion != database.MaxSchemaVersion() {
		return fmt.Errorf("running mailbox helper is incompatible (bootstrap protocol %d, schema %d/%d); stop or update the helper and retry", status.ProtocolVersion, status.SchemaVersion, status.MaxSchemaVersion)
	}
	return nil
}

func (a *App) runMailboxHelperCommand(args ...string) ([]byte, error) {
	helperPath, err := a.mailboxHelperPath()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.helperCommandContext(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, helperPath, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("mailbox helper %s failed: %s: %w", args[0], strings.TrimSpace(string(output)), err)
	}
	return output, nil
}

func (a *App) helperCommandContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// EnableMailboxServiceWrites explicitly replaces the desktop's read-only service grant with one that can change mail flags.
func (a *App) EnableMailboxServiceWrites() (MailboxServiceStatus, error) {
	if a.mailboxService == nil {
		return MailboxServiceStatus{ErrorCode: "service_unavailable", Message: "The local mailbox service is not reachable."}, errors.New("mailbox service is not initialized")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.mailboxService.authorizeWrites(ctx); err != nil {
		return a.mailboxService.status(ctx), err
	}
	return a.mailboxService.status(ctx), nil
}
