package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"

	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/oauth2"
	"github.com/hkdb/aerion/internal/platform"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// NativeGmailReviewRequest pins every UI operation to the opened workspace.
type NativeGmailReviewRequest struct {
	Workspace   string           `json:"workspace"`
	Operation   string           `json:"operation"`
	AccountID   string           `json:"accountId"`
	RuleID      string           `json:"ruleId"`
	ID          string           `json:"id"`
	Fingerprint string           `json:"fingerprint"`
	Draft       NativeGmailDraft `json:"draft"`
}

// NativeGmailReviewReply uses the same review/recovery UI for either owner.
type NativeGmailReviewReply struct {
	Context     *NativeGmailContext           `json:"context,omitempty"`
	Preview     *NativeGmailPreview           `json:"preview,omitempty"`
	Publication *NativeGmailPublicationStatus `json:"publication,omitempty"`
}

// GetNativeGmailWorkspace captures the configured execution owner without any
// offline-helper fallback. The returned value must accompany every UI action.
type NativeGmailWorkspace struct {
	Key      string `json:"key"`
	Target   string `json:"target"`
	Endpoint string `json:"endpoint"`
}

func (a *App) GetNativeGmailWorkspace() (NativeGmailWorkspace, error) {
	_, workspace, err := a.captureNativeGmailWorkspace()
	return workspace, err
}
func (a *App) captureNativeGmailWorkspace() (*automationclient.Client, NativeGmailWorkspace, error) {
	connector := a.mailboxService
	if connector == nil {
		return nil, NativeGmailWorkspace{Key: "standalone", Target: "local"}, nil
	}
	client, err := connector.getClient()
	if err != nil {
		return nil, NativeGmailWorkspace{}, err
	}
	connector.mu.Lock()
	defer connector.mu.Unlock()
	endpoint := connector.endpoint
	if endpoint == "" {
		endpoint = mailboxServiceEndpoint
	}
	if connector != a.mailboxService || connector.client != client {
		return nil, NativeGmailWorkspace{}, fmt.Errorf("filter workspace changed; reopen Gmail review")
	}
	binding := sha256.Sum256([]byte(fmt.Sprintf("%p/%p/%s", connector, client, endpoint)))
	return client, NativeGmailWorkspace{Key: fmt.Sprintf("helper:%x", binding), Target: "helper", Endpoint: endpoint}, nil
}
func (a *App) nativeGmailWorkspaceClient(workspace string) (*automationclient.Client, error) {
	client, current, err := a.captureNativeGmailWorkspace()
	if err != nil {
		return nil, err
	}
	if workspace == "" || workspace != current.Key {
		return nil, fmt.Errorf("filter workspace changed; reopen Gmail review")
	}
	return client, nil
}

// NativeGmailReview routes only to the workspace the UI opened. A helper error
// never reads local rule/credential stores and never retries a create request.
func (a *App) NativeGmailReview(req NativeGmailReviewRequest) (NativeGmailReviewReply, error) {
	result := NativeGmailReviewReply{}
	client, err := a.nativeGmailWorkspaceClient(req.Workspace)
	if err != nil {
		return result, err
	}
	if client == nil {
		return a.localNativeGmailReview(req)
	}
	call := v1.GmailPublicationRequest{Operation: req.Operation, AccountID: req.AccountID, RuleID: req.RuleID, ID: req.ID, Fingerprint: req.Fingerprint}
	switch req.Operation {
	case "preview", "prepare":
		if req.Draft.RuleID != req.RuleID {
			return result, fmt.Errorf("draft source changed")
		}
		call.Filter, err = nativeGmailPayload(req.Draft)
		if err != nil {
			return result, err
		}
	case "context", "status", "publish", "reconcile", "cancel":
	default:
		return result, fmt.Errorf("unknown Gmail review operation")
	}
	reply, err := client.GmailPublication(a.helperCommandContext(), call)
	if reply.Publication != nil {
		status := helperPublicationStatus(reply.Publication, reply.Fingerprint)
		result.Publication = &status
	}
	if err != nil {
		return result, err
	}
	switch req.Operation {
	case "context":
		context := NativeGmailContext{AccountID: reply.AccountID, Email: reply.Email, ConsentRequired: reply.ConsentRequired, Labels: []NativeGmailLabel{}}
		for _, label := range reply.Labels {
			context.Labels = append(context.Labels, NativeGmailLabel{ID: label.ID, Name: label.Name})
		}
		result.Context = &context
	case "preview":
		result.Preview = &NativeGmailPreview{AccountEmail: reply.Email, Found: reply.Found, Truncated: reply.Truncated, Fingerprint: reply.Fingerprint}
	}
	return result, nil
}
func helperPublicationStatus(plan *mailfilter.GmailPublication, fingerprint string) NativeGmailPublicationStatus {
	status := publicationStatus(plan)
	status.Fingerprint = fingerprint
	return status
}
func (a *App) localNativeGmailReview(req NativeGmailReviewRequest) (NativeGmailReviewReply, error) {
	result := NativeGmailReviewReply{}
	switch req.Operation {
	case "context":
		value, err := a.GetNativeGmailContext(req.RuleID)
		result.Context = &value
		return result, err
	case "status":
		value, err := a.GetNativeGmailPublication(req.RuleID)
		if value.ID != "" {
			result.Publication = &value
		}
		return result, err
	case "preview":
		if req.Draft.RuleID != req.RuleID {
			return result, fmt.Errorf("draft source changed")
		}
		value, err := a.PreviewNativeGmailFilter(req.Draft)
		result.Preview = &value
		return result, err
	case "prepare":
		if req.Draft.RuleID != req.RuleID {
			return result, fmt.Errorf("draft source changed")
		}
		value, err := a.PrepareNativeGmailFilter(req.Draft, req.Fingerprint)
		result.Publication = &value
		return result, err
	case "publish":
		value, err := a.PublishNativeGmailFilter(req.ID, req.Fingerprint)
		result.Publication = &value
		return result, err
	case "reconcile":
		value, err := a.ReconcileNativeGmailFilter(req.ID)
		result.Publication = &value
		return result, err
	case "cancel":
		return result, a.CancelNativeGmailFilter(req.ID)
	default:
		return result, fmt.Errorf("unknown Gmail review operation")
	}
}

// AuthorizeNativeGmailWorkspace uses a desktop loopback listener but keeps the
// helper PKCE, code exchange and saved credentials on the selected helper.
func (a *App) AuthorizeNativeGmailWorkspace(workspace, accountID string) error {
	return a.authorizeNativeGmailWorkspace(workspace, accountID, func(email, authURL string) {
		wailsRuntime.EventsEmit(a.ctx, "gmail-filter-oauth:started", map[string]interface{}{"accountId": accountID, "email": email, "authURL": authURL})
		if err := platform.PortalOpenURI(authURL); err != nil {
			wailsRuntime.BrowserOpenURL(a.ctx, authURL)
		}
	})
}
func (a *App) authorizeNativeGmailWorkspace(workspace, accountID string, openURL func(string, string)) error {
	client, err := a.nativeGmailWorkspaceClient(workspace)
	if err != nil {
		return err
	}
	if client == nil {
		return a.AuthorizeGmailFilters(accountID)
	}
	if !a.gmailFilterOAuthMu.TryLock() {
		return fmt.Errorf("Gmail filter authorization already in progress")
	}
	defer a.gmailFilterOAuthMu.Unlock()

	if a.ctx == nil {
		return fmt.Errorf("application context unavailable")
	}
	listener := oauth2.NewCallbackServer()
	port, err := listener.Start(a.ctx, "127.0.0.1")
	if err != nil {
		return err
	}
	defer listener.Stop()
	reply, err := client.GmailConsent(a.helperCommandContext(), v1.GmailConsentRequest{Operation: "start", AccountID: accountID, RedirectURI: fmt.Sprintf("http://127.0.0.1:%d/callback", port)})
	if err != nil {
		return err
	}
	if reply.Authorized {
		if reply.AccountID != accountID {
			return fmt.Errorf("helper consent selected account mismatch")
		}
		return nil
	}
	if reply.SessionID == "" || reply.AuthURL == "" || reply.AccountID != accountID {
		return fmt.Errorf("helper consent response incomplete")
	}
	defer func() {
		_, _ = client.GmailConsent(context.Background(), v1.GmailConsentRequest{Operation: "cancel", AccountID: accountID, SessionID: reply.SessionID})
	}()
	// Verify the returned URL is the expected Google authorization origin before
	// opening it. The helper owns the project and scopes, not a desktop guess.
	u, parseErr := url.Parse(reply.AuthURL)
	if parseErr != nil || u.Scheme != "https" || u.Host != "accounts.google.com" || u.Path != "/o/oauth2/v2/auth" || u.User != nil || u.Fragment != "" || u.Query().Get("redirect_uri") != fmt.Sprintf("http://127.0.0.1:%d/callback", port) || u.Query().Get("state") == "" {
		return fmt.Errorf("unexpected helper authorization context")
	}

	openURL(reply.Email, reply.AuthURL)
	callback, err := listener.WaitForCallback(a.ctx)
	if err != nil {
		return err
	}
	if callback.Error != "" {
		return fmt.Errorf("Google authorization was not completed")
	}
	if _, err = a.nativeGmailWorkspaceClient(workspace); err != nil {
		return err
	}
	completed, err := client.GmailConsent(a.helperCommandContext(), v1.GmailConsentRequest{Operation: "complete", AccountID: accountID, SessionID: reply.SessionID, Code: callback.Code, State: callback.State})
	if err != nil {
		return fmt.Errorf("helper consent result unverified; reopen review to check account context: %w", err)
	}
	if !completed.Authorized || completed.AccountID != accountID {
		return fmt.Errorf("helper did not verify selected account consent")
	}
	return nil
}
