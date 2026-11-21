package v1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// GmailConsentRequest carries an explicit helper-owned consent lifecycle action.
// Callback tokens/PKCE secrets are never accepted, only a one-time code/state.
type GmailConsentRequest struct {
	Operation   string `json:"operation"`
	AccountID   string `json:"accountId"`
	SessionID   string `json:"sessionId,omitempty"`
	RedirectURI string `json:"redirectUri,omitempty"`
	Code        string `json:"code,omitempty"`
	State       string `json:"state,omitempty"`
}

// GmailConsentReply contains only the browser URL and non-secret account context.
type GmailConsentReply struct {
	SessionID  string `json:"sessionId,omitempty"`
	AccountID  string `json:"accountId"`
	Email      string `json:"email"`
	AuthURL    string `json:"authUrl,omitempty"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	Authorized bool   `json:"authorized"`
}

// GmailConsentOperations holds PKCE and persists grants only on the helper.
type GmailConsentOperations interface {
	GmailConsent(context.Context, string, GmailConsentRequest) (GmailConsentReply, error)
}

// MountGmailConsentRoutes requires account administration for every stage and
// binds the session to the authenticated client, never an ID in request JSON.
func MountGmailConsentRoutes(mux RouteMux, ops GmailConsentOperations, clientID func(*http.Request) string, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/filters/gmail/consent", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) || !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		if r.Method != http.MethodPost {
			writeFilterRuleError(w, r, 405, CodeInvalidRequest, "Method not allowed")
			return
		}
		if ops == nil || clientID == nil || clientID(r) == "" {
			writeFilterRuleError(w, r, 503, CodeInternal, "Helper consent owner unavailable")
			return
		}
		var req GmailConsentRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "Malformed consent request")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "One consent request required")
			return
		}
		switch req.Operation {
		case "start", "complete", "cancel":
		default:
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "Unknown consent operation")
			return
		}
		if strings.TrimSpace(req.AccountID) == "" {
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "Choose a Gmail account before authorization")
			return
		}
		result, err := ops.GmailConsent(r.Context(), clientID(r), req)
		if err != nil {
			writeFilterRuleError(w, r, 409, CodeConflict, "Gmail consent could not be completed. Reopen authorization, select the Gmail account shown in this review, and approve the Gmail settings permission. If it still fails, check the account owner and connection before retrying.")
			return
		}
		writeFilterRuleResponse(w, r, result)
	})
}
