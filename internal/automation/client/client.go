package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/replylater"
	"github.com/hkdb/aerion/internal/smtp"
	"github.com/hkdb/aerion/internal/triage"
	"github.com/hkdb/aerion/internal/views"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

// Client interacts with Postilaatikko's local automation API.
type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

// Config configures a Client instance.
type Config struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
}

// New creates an automation Client.
func New(cfg Config) *Client {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8029"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		endpoint: endpoint,
		token:    cfg.Token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any, idempotencyKey string) ([]byte, error) {
	return c.doRequestWithHeaders(ctx, method, path, body, idempotencyKey, nil)
}

func (c *Client) doRequestWithHeaders(ctx context.Context, method, path string, body any, idempotencyKey string, headers map[string]string) ([]byte, error) {
	var bodyReader io.Reader
	var requestBytes []byte
	defer func() { clear(requestBytes) }()
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		requestBytes = data
		bodyReader = bytes.NewReader(requestBytes)
	}

	url := c.endpoint + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp v1.ErrorResponse
		if jsonErr := json.Unmarshal(respBytes, &errResp); jsonErr == nil && errResp.Error.Code != "" {
			return nil, fmt.Errorf("api error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return nil, fmt.Errorf("api error (http %d): %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}

// GetCapabilities returns authorized capabilities for this token.
func (c *Client) GetCapabilities(ctx context.Context) (*v1.Capabilities, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/capabilities", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.Capabilities]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetAccounts lists configured accounts.
func (c *Client) GetAccounts(ctx context.Context) ([]v1.AccountSummary, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/accounts", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]v1.AccountSummary]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetAccount retrieves one account's non-secret configuration by ID.
func (c *Client) GetAccount(ctx context.Context, accountID string) (*v1.AccountSummary, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/accounts/"+url.PathEscape(accountID), nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.AccountSummary]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetIdentities lists the account's non-secret sender identities and signatures.
func (c *Client) GetIdentities(ctx context.Context, accountID string) ([]*account.Identity, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/accounts/"+url.PathEscape(accountID)+"/identities", nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[[]*account.Identity]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// CreateAccount creates account configuration with an optional password envelope.
func (c *Client) CreateAccount(ctx context.Context, mutation v1.AccountMutation, credentials v1.PasswordCredentialMutation, idempotencyKey string) (*v1.AccountSummary, error) {
	mutation.Credentials = &credentials
	return c.accountMutation(ctx, http.MethodPost, "/v1/accounts", mutation, idempotencyKey)
}

// UpdateAccount changes account configuration with an optional password envelope.
func (c *Client) UpdateAccount(ctx context.Context, accountID string, mutation v1.AccountMutation, credentials v1.PasswordCredentialMutation, idempotencyKey string) (*v1.AccountSummary, error) {
	mutation.Credentials = &credentials
	return c.accountMutation(ctx, http.MethodPut, "/v1/accounts/"+url.PathEscape(accountID), mutation, idempotencyKey)
}

// SetAccountOAuthCredentials sends a one-time OAuth callback package to the helper.
func (c *Client) SetAccountOAuthCredentials(ctx context.Context, accountID string, mutation v1.OAuthCredentialMutation, idempotencyKey string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/accounts/"+url.PathEscape(accountID)+"/oauth", mutation, idempotencyKey)
	return err
}

// ListDrafts returns the bounded set of drafts for one helper-owned account.
func (c *Client) ListDrafts(ctx context.Context, accountID string) ([]v1.DraftDocument, error) {
	query := url.Values{"accountId": []string{accountID}}
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/drafts?"+query.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[[]v1.DraftDocument]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// GetDraft returns one account-qualified draft and its compare-and-swap revision.
func (c *Client) GetDraft(ctx context.Context, accountID, draftID string) (*v1.DraftDocument, error) {
	query := url.Values{"accountId": []string{accountID}}
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/drafts/"+url.PathEscape(draftID)+"?"+query.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[v1.DraftDocument]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// CreateDraft creates a helper-owned draft.
func (c *Client) CreateDraft(ctx context.Context, mutation v1.DraftMutation) (*v1.DraftDocument, error) {
	return c.draftMutation(ctx, http.MethodPost, "/v1/drafts", mutation)
}

// UpdateDraft replaces a draft only when its previously-read revision is current.
func (c *Client) UpdateDraft(ctx context.Context, draftID string, mutation v1.DraftMutation) (*v1.DraftDocument, error) {
	return c.draftMutation(ctx, http.MethodPut, "/v1/drafts/"+url.PathEscape(draftID), mutation)
}

func (c *Client) draftMutation(ctx context.Context, method, path string, mutation v1.DraftMutation) (*v1.DraftDocument, error) {
	data, err := c.doRequest(ctx, method, path, mutation, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[v1.DraftDocument]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// DeleteDraft removes one draft only when its revision is current.
func (c *Client) DeleteDraft(ctx context.Context, accountID, draftID, revision string) error {
	query := url.Values{"accountId": []string{accountID}}
	_, err := c.doRequestWithHeaders(ctx, http.MethodDelete, "/v1/drafts/"+url.PathEscape(draftID)+"?"+query.Encode(), nil, "", map[string]string{"If-Match": revision})
	return err
}

// PrepareReply asks the helper to build reply/reply-all/forward context from a cached message.
func (c *Client) PrepareReply(ctx context.Context, request v1.ReplyPreparationRequest) (*smtp.ComposeMessage, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/v1/replies", request, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[smtp.ComposeMessage]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// SendPreparedMessage sends already prepared MIME through the helper exactly once per key.
func (c *Client) SendPreparedMessage(ctx context.Context, request v1.SendRequest, idempotencyKey string) (*v1.SendResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/v1/send", request, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var response v1.Response[v1.SendResponse]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// GetSendReceipt reads the durable result for an existing send idempotency key.
func (c *Client) GetSendReceipt(ctx context.Context, idempotencyKey string) (*v1.SendReceiptLookup, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/send/receipts/"+url.PathEscape(idempotencyKey), nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[v1.SendReceiptLookup]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

func (c *Client) accountMutation(ctx context.Context, method, path string, mutation any, idempotencyKey string) (*v1.AccountSummary, error) {
	bytes, err := c.doRequest(ctx, method, path, mutation, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.AccountSummary]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DeleteAccount removes an account and its helper-local credentials.
func (c *Client) DeleteAccount(ctx context.Context, accountID, idempotencyKey string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/v1/accounts/"+url.PathEscape(accountID), nil, idempotencyKey)
	return err
}

// SetAccountEnabled updates one account's sync-enabled state.
func (c *Client) SetAccountEnabled(ctx context.Context, accountID string, enabled bool, idempotencyKey string) (*v1.AccountSummary, error) {
	bytes, err := c.doRequest(ctx, http.MethodPatch, "/v1/accounts/"+url.PathEscape(accountID)+"/enabled", v1.AccountEnabledMutation{Enabled: enabled}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.AccountSummary]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ReorderAccounts applies a complete account order.
func (c *Client) ReorderAccounts(ctx context.Context, ids []string, idempotencyKey string) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/v1/accounts/order", v1.AccountOrderMutation{IDs: ids}, idempotencyKey)
	return err
}

// AddSharedMailbox creates a Microsoft shared mailbox under its primary account.
func (c *Client) AddSharedMailbox(ctx context.Context, mutation v1.SharedMailboxMutation, idempotencyKey string) (*v1.AccountSummary, error) {
	return c.accountMutation(ctx, http.MethodPost, "/v1/accounts/shared-mailboxes", mutation, idempotencyKey)
}

// GetFolders lists folders, optionally filtered by account.
func (c *Client) GetFolders(ctx context.Context, accountID string) ([]v1.FolderSummary, error) {
	path := "/v1/folders"
	if accountID != "" {
		path += "?accountId=" + accountID
	}
	bytes, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]v1.FolderSummary]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// SyncFolders asks the helper to refresh one account's provider folder catalog.
func (c *Client) SyncFolders(ctx context.Context, accountID string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/folders/sync", v1.FolderSyncRequest{AccountID: accountID}, "")
	return err
}

// SetFolderSubscription updates one account-qualified provider folder subscription.
func (c *Client) SetFolderSubscription(ctx context.Context, accountID, folderID string, subscribed bool) error {
	subscribedValue := subscribed
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/folders/subscription", v1.FolderSubscriptionRequest{
		AccountID: accountID, FolderID: folderID, Subscribed: &subscribedValue,
	}, "")
	return err
}

// SetAllFolderSubscriptions updates all provider folder subscriptions for an account.
func (c *Client) SetAllFolderSubscriptions(ctx context.Context, accountID string, subscribed bool) error {
	subscribedValue := subscribed
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/folders/subscriptions", v1.FolderSubscriptionsRequest{
		AccountID: accountID, Subscribed: &subscribedValue,
	}, "")
	return err
}

// GetViews lists saved views/virtual folders.
func (c *Client) GetViews(ctx context.Context) ([]v1.ViewSummary, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/views", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]v1.ViewSummary]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// MutateVirtualFolder performs one helper-owned saved-view operation.
func (c *Client) MutateVirtualFolder(ctx context.Context, req v1.VirtualFolderMutationRequest) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/views/mutations", req, "")
	return err
}

// VirtualFolders lists the complete saved-view definitions held by the service.
func (c *Client) VirtualFolders(ctx context.Context) ([]*virtualfolder.VirtualFolder, error) {
	views, err := c.GetViews(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*virtualfolder.VirtualFolder, 0, len(views))
	for _, view := range views {
		result = append(result, &virtualfolder.VirtualFolder{
			ID: view.ID, Name: view.Name, OrderIndex: view.OrderIndex, FolderIDs: view.FolderIDs,
			FolderMode: view.FolderMode, ReadStatus: view.ReadStatus, ReplyStatus: view.ReplyStatus,
			StarStatus: view.StarStatus, DateWindow: view.DateWindow, SearchQuery: view.SearchQuery,
			CreatedAt: view.CreatedAt, UpdatedAt: view.UpdatedAt,
		})
	}
	return result, nil
}

// VirtualFolderUnreadCounts returns helper-owned saved-view sidebar counts.
func (c *Client) VirtualFolderUnreadCounts(ctx context.Context) (map[string]int, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/views/unread-counts", nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[map[string]int]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return map[string]int{}, nil
	}
	return response.Data, nil
}

// CachedVirtualFolderView retrieves the helper-owned, hydrated saved-view cache.
func (c *Client) CachedVirtualFolderView(ctx context.Context, viewID string) (*v1.CachedVirtualFolderView, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/views/"+url.PathEscape(viewID)+"/cache", nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[v1.CachedVirtualFolderView]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// GetFilterRules lists service-owned automatic filter rules.
func (c *Client) GetFilterRules(ctx context.Context) ([]*mailfilter.Rule, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/filters", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]*mailfilter.Rule]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// MutateFilterRule performs one helper-owned filter-rule operation.
func (c *Client) MutateFilterRule(ctx context.Context, req v1.FilterRuleMutationRequest) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/filters/mutations", req, "")
	return err
}

// GetUncertainFilterJobs lists one bounded page of automatic moves for inspection.
func (c *Client) GetUncertainFilterJobs(ctx context.Context, limit int, cursor string) (v1.FilterJobPage, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	path := "/v1/filter-jobs"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	bytes, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return v1.FilterJobPage{}, err
	}
	var resp v1.Response[v1.FilterJobPage]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return v1.FilterJobPage{}, err
	}
	return resp.Data, nil
}

// GetReplyLaterQueue lists helper-owned Reply Later entries.
func (c *Client) GetReplyLaterQueue(ctx context.Context) ([]replylater.Ref, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/reply-later", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]replylater.Ref]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// SetReplyLaterMembership updates one account-qualified conversation's queue state.
func (c *Client) SetReplyLaterMembership(ctx context.Context, ref replylater.Ref, inQueue bool) (bool, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/reply-later/mutations", v1.ReplyLaterMutationRequest{Ref: ref, InQueue: inQueue}, "")
	if err != nil {
		return false, err
	}
	var resp v1.Response[v1.ReplyLaterMutationResponse]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return false, err
	}
	return resp.Data.Changed, nil
}

// GetReadLaterQueue lists helper-owned Read Later membership.
func (c *Client) GetReadLaterQueue(ctx context.Context) ([]triage.Ref, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/triage/read-later", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]triage.Ref]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// SetReadLater changes one account-qualified Read Later membership.
func (c *Client) SetReadLater(ctx context.Context, ref triage.Ref, inQueue bool) (bool, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/triage/read-later/mutations", v1.ReadLaterMutationRequest{Ref: ref, InQueue: inQueue}, "")
	if err != nil {
		return false, err
	}
	var resp v1.Response[v1.TriageMutationResponse]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return false, err
	}
	return resp.Data.Changed, nil
}

// GetSnoozedConversations lists active helper-owned Snoozes.
func (c *Client) GetSnoozedConversations(ctx context.Context) ([]triage.Ref, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/triage/snoozes", nil, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[[]triage.Ref]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetSnoozedCount returns the number of active helper-owned Snoozes.
func (c *Client) GetSnoozedCount(ctx context.Context) (int, error) {
	bytes, err := c.doRequest(ctx, http.MethodGet, "/v1/triage/snoozes/count", nil, "")
	if err != nil {
		return 0, err
	}
	var resp v1.Response[int]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return 0, err
	}
	return resp.Data, nil
}

// SnoozeConversation adds or updates a helper-owned Snooze deadline.
func (c *Client) SnoozeConversation(ctx context.Context, ref triage.Ref, wakeAt string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/v1/triage/snoozes/mutations", v1.SnoozeMutationRequest{Operation: "snooze", Ref: ref, WakeAt: wakeAt}, "")
	return err
}

// UnsnoozeConversation removes one helper-owned Snooze.
func (c *Client) UnsnoozeConversation(ctx context.Context, ref triage.Ref) (bool, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/triage/snoozes/mutations", v1.SnoozeMutationRequest{Operation: "unsnooze", Ref: ref}, "")
	if err != nil {
		return false, err
	}
	var resp v1.Response[v1.TriageMutationResponse]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return false, err
	}
	return resp.Data.Changed, nil
}

// Search queries messages across a given view scope.
func (c *Client) Search(ctx context.Context, req v1.SearchRequest) (*views.ThreadPage, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/search", req, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[views.ThreadPage]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListConversations retrieves a bounded header-only page for a folder or all inboxes.
func (c *Client) ListConversations(ctx context.Context, req v1.ConversationListRequest) (*v1.ConversationListPage, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/conversations", req, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.ConversationListPage]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ReadThread retrieves thread messages and metadata.
func (c *Client) ReadThread(ctx context.Context, req v1.ThreadReadRequest) (*views.ThreadView, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/threads/read", req, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[views.ThreadView]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetMessage returns account-qualified message metadata without the body.
func (c *Client) GetMessage(ctx context.Context, accountID, messageID string) (*message.Message, error) {
	query := url.Values{"accountId": []string{accountID}}
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/messages/"+url.PathEscape(messageID)+"/header?"+query.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[message.Message]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// ReadBody reads or explicitly fetches message body and extracted reply content.
func (c *Client) ReadBody(ctx context.Context, req v1.BodyReadRequest) (*v1.MessageBody, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/messages/body", req, "")
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.MessageBody]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// MutateAction executes a mutation with an Idempotency-Key.
func (c *Client) MutateAction(ctx context.Context, req v1.ActionRequest, idempotencyKey string) (*v1.ActionResponse, error) {
	bytes, err := c.doRequest(ctx, http.MethodPost, "/v1/actions", req, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var resp v1.Response[v1.ActionResponse]
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
