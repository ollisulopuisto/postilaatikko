package v1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
)

var (
	ErrAccountMutationKeyConflict = errors.New("account mutation idempotency key conflict")
	ErrAccountMutationInProgress  = errors.New("account mutation is already in progress")
)

// AccountMutationLedger persists non-secret idempotency receipts for account mutations.
type AccountMutationLedger interface {
	Reserve(context.Context, string, string) ([]byte, int, error)
	Complete(context.Context, string, string, int, []byte) error
}

var ErrCredentialRepairRequired = errors.New("account credential change requires repair")

// PasswordAction selects how one stored account password is handled.
type PasswordAction string

const (
	// PasswordKeep leaves the stored value unchanged.
	PasswordKeep PasswordAction = "keep"
	// PasswordReplace stores the supplied replacement value.
	PasswordReplace PasswordAction = "replace"
	// PasswordClear removes the stored value.
	PasswordClear PasswordAction = "clear"
)

// PasswordCredentialMutation carries one-time account password actions.
// Secret values are accepted only in requests and are never returned.
type PasswordCredentialMutation struct {
	IMAPAction   PasswordAction `json:"imapAction,omitempty"`
	IMAPPassword string         `json:"imapPassword,omitempty"`
	SMTPAction   PasswordAction `json:"smtpAction,omitempty"`
	SMTPPassword string         `json:"smtpPassword,omitempty"`
}

// OAuthCredentialMutation carries one-time callback credentials to the helper.
// Token and provider-secret values are accepted only on authenticated writes.
type OAuthCredentialMutation struct {
	Provider     string               `json:"provider"`
	AccessToken  string               `json:"accessToken"`
	RefreshToken string               `json:"refreshToken"`
	ExpiresAt    time.Time            `json:"expiresAt"`
	Scopes       []string             `json:"scopes"`
	StableID     string               `json:"stableId,omitempty"`
	Custom       *CustomOAuthProvider `json:"customProvider,omitempty"`
}

// CustomOAuthProvider contains the account-scoped configuration needed for
// custom-provider token refresh and reauthorization.
type CustomOAuthProvider struct {
	AuthURL          string   `json:"authUrl"`
	TokenURL         string   `json:"tokenUrl"`
	UserinfoEndpoint string   `json:"userinfoEndpoint,omitempty"`
	Scopes           []string `json:"scopes"`
	ClientID         string   `json:"clientId"`
	ClientSecret     string   `json:"clientSecret"`
}

func (m *OAuthCredentialMutation) Normalize() error {
	if m == nil || m.Provider == "" || m.AccessToken == "" || m.ExpiresAt.IsZero() {
		return InvalidRequest("OAuth provider, access token, and expiry are required", nil)
	}
	if m.Provider == "custom" {
		if m.Custom == nil || m.Custom.AuthURL == "" || m.Custom.TokenURL == "" || m.Custom.ClientID == "" {
			return InvalidRequest("Custom OAuth provider configuration is incomplete", nil)
		}
		return nil
	}
	if m.Custom != nil {
		return InvalidRequest("Custom provider configuration is only valid for the custom provider", nil)
	}
	return nil
}

func (m *OAuthCredentialMutation) ClearSecrets() {
	if m == nil {
		return
	}
	m.AccessToken, m.RefreshToken = "", ""
	if m.Custom != nil {
		m.Custom.ClientSecret = ""
	}
}

func (m *PasswordCredentialMutation) Normalize() error {
	if m == nil {
		return nil
	}
	if m.IMAPAction == "" {
		m.IMAPAction = PasswordKeep
	}
	if m.SMTPAction == "" {
		m.SMTPAction = PasswordKeep
	}
	for _, field := range []struct {
		action   PasswordAction
		password string
		label    string
	}{{m.IMAPAction, m.IMAPPassword, "IMAP"}, {m.SMTPAction, m.SMTPPassword, "SMTP"}} {
		switch field.action {
		case PasswordKeep, PasswordClear:
			if field.password != "" {
				return InvalidRequest(field.label+" password must be omitted unless replacing it", nil)
			}
		case PasswordReplace:
			if field.password == "" {
				return InvalidRequest(field.label+" password is required for replacement", nil)
			}
		default:
			return InvalidRequest(field.label+" password action is invalid", nil)
		}
	}
	return nil
}

func (m PasswordCredentialMutation) HasChanges() bool {
	return m.IMAPAction == PasswordReplace || m.IMAPAction == PasswordClear || m.SMTPAction == PasswordReplace || m.SMTPAction == PasswordClear
}

func (m *PasswordCredentialMutation) ClearSecrets() {
	if m == nil {
		return
	}
	m.IMAPPassword = ""
	m.SMTPPassword = ""
}

// AccountMutationOperations applies account configuration changes in the helper.
type AccountMutationOperations interface {
	CreateAccount(AccountMutation, PasswordCredentialMutation) (*account.Account, error)
	UpdateAccount(string, AccountMutation, PasswordCredentialMutation) (*account.Account, error)
	DeleteAccount(string) error
	SetAccountEnabled(string, bool) (*account.Account, error)
	ReorderAccounts([]string) error
	AddSharedMailbox(context.Context, SharedMailboxMutation) (*account.Account, error)
}

// AccountOAuthMutationOperations accepts helper-owned OAuth callback material.
type AccountOAuthMutationOperations interface {
	SetAccountOAuthCredentials(string, OAuthCredentialMutation) error
}

// AccountMutation contains public account settings and an optional one-time
// password envelope. Password values are never returned or persisted in receipts.
type AccountMutation struct {
	Credentials              *PasswordCredentialMutation `json:"credentials,omitempty"`
	Name                     string                      `json:"name"`
	DisplayName              string                      `json:"displayName"`
	Email                    string                      `json:"email"`
	IMAPHost                 string                      `json:"imapHost"`
	IMAPPort                 int                         `json:"imapPort"`
	IMAPSecurity             account.SecurityType        `json:"imapSecurity"`
	IMAPAuthMechanism        account.AuthMechanism       `json:"imapAuthMechanism"`
	SMTPHost                 string                      `json:"smtpHost"`
	SMTPPort                 int                         `json:"smtpPort"`
	SMTPSecurity             account.SecurityType        `json:"smtpSecurity"`
	SMTPAuthMechanism        account.AuthMechanism       `json:"smtpAuthMechanism"`
	NoOutgoingServer         bool                        `json:"noOutgoingServer"`
	SMTPUsername             string                      `json:"smtpUsername"`
	ReplyForwardIdentityID   string                      `json:"replyForwardIdentityId"`
	AuthType                 account.AuthType            `json:"authType"`
	Username                 string                      `json:"username"`
	Color                    string                      `json:"color"`
	SyncPeriodDays           int                         `json:"syncPeriodDays"`
	SyncInterval             int                         `json:"syncInterval"`
	SyncAllFolders           bool                        `json:"syncAllFolders"`
	SyncFoldersEnabled       bool                        `json:"syncFoldersEnabled"`
	ReadReceiptRequestPolicy string                      `json:"readReceiptRequestPolicy"`
	SentFolderPath           string                      `json:"sentFolderPath,omitempty"`
	DraftsFolderPath         string                      `json:"draftsFolderPath,omitempty"`
	TrashFolderPath          string                      `json:"trashFolderPath,omitempty"`
	SpamFolderPath           string                      `json:"spamFolderPath,omitempty"`
	ArchiveFolderPath        string                      `json:"archiveFolderPath,omitempty"`
	AllMailFolderPath        string                      `json:"allMailFolderPath,omitempty"`
	StarredFolderPath        string                      `json:"starredFolderPath,omitempty"`
}

// ToAccountConfig maps public non-secret fields to the existing account-store configuration.
func (m AccountMutation) ToAccountConfig() account.AccountConfig {
	return account.AccountConfig{
		Name: m.Name, DisplayName: m.DisplayName, Email: m.Email,
		IMAPHost: m.IMAPHost, IMAPPort: m.IMAPPort, IMAPSecurity: m.IMAPSecurity, IMAPAuthMechanism: m.IMAPAuthMechanism,
		SMTPHost: m.SMTPHost, SMTPPort: m.SMTPPort, SMTPSecurity: m.SMTPSecurity, SMTPAuthMechanism: m.SMTPAuthMechanism,
		NoOutgoingServer: m.NoOutgoingServer, SMTPUsername: m.SMTPUsername, ReplyForwardIdentityID: m.ReplyForwardIdentityID,
		AuthType: m.AuthType, Username: m.Username, Color: m.Color, SyncPeriodDays: m.SyncPeriodDays,
		SyncInterval: m.SyncInterval, SyncAllFolders: m.SyncAllFolders, SyncFoldersEnabled: m.SyncFoldersEnabled,
		ReadReceiptRequestPolicy: m.ReadReceiptRequestPolicy, SentFolderPath: m.SentFolderPath,
		DraftsFolderPath: m.DraftsFolderPath, TrashFolderPath: m.TrashFolderPath, SpamFolderPath: m.SpamFolderPath,
		ArchiveFolderPath: m.ArchiveFolderPath, AllMailFolderPath: m.AllMailFolderPath, StarredFolderPath: m.StarredFolderPath,
	}
}

// AccountMutationFromConfig copies only public account settings from a desktop config.
func AccountMutationFromConfig(c account.AccountConfig) AccountMutation {
	return AccountMutation{
		Name: c.Name, DisplayName: c.DisplayName, Email: c.Email,
		IMAPHost: c.IMAPHost, IMAPPort: c.IMAPPort, IMAPSecurity: c.IMAPSecurity, IMAPAuthMechanism: c.IMAPAuthMechanism,
		SMTPHost: c.SMTPHost, SMTPPort: c.SMTPPort, SMTPSecurity: c.SMTPSecurity, SMTPAuthMechanism: c.SMTPAuthMechanism,
		NoOutgoingServer: c.NoOutgoingServer, SMTPUsername: c.SMTPUsername, ReplyForwardIdentityID: c.ReplyForwardIdentityID,
		AuthType: c.AuthType, Username: c.Username, Color: c.Color, SyncPeriodDays: c.SyncPeriodDays,
		SyncInterval: c.SyncInterval, SyncAllFolders: c.SyncAllFolders, SyncFoldersEnabled: c.SyncFoldersEnabled,
		ReadReceiptRequestPolicy: c.ReadReceiptRequestPolicy, SentFolderPath: c.SentFolderPath,
		DraftsFolderPath: c.DraftsFolderPath, TrashFolderPath: c.TrashFolderPath, SpamFolderPath: c.SpamFolderPath,
		ArchiveFolderPath: c.ArchiveFolderPath, AllMailFolderPath: c.AllMailFolderPath, StarredFolderPath: c.StarredFolderPath,
	}
}

// SharedMailboxMutation requests a child account linked to an existing Microsoft account.
type SharedMailboxMutation struct {
	PrimaryAccountID string `json:"primaryAccountId"`
	Email            string `json:"email"`
	DisplayName      string `json:"displayName"`
}

type AccountEnabledMutation struct {
	Enabled bool `json:"enabled"`
}

type AccountOrderMutation struct {
	IDs []string `json:"ids"`
}

type AccountMutationHandler struct {
	operations  AccountMutationOperations
	ledger      AccountMutationLedger
	reader      ReadHandler
	fingerprint func([]byte) []byte
}

// NewAccountMutationHandler creates the helper's authenticated account mutation handler.
func NewAccountMutationHandler(operations AccountMutationOperations, ledger AccountMutationLedger, fingerprinters ...func([]byte) []byte) *AccountMutationHandler {
	handler := &AccountMutationHandler{operations: operations, ledger: ledger}
	if len(fingerprinters) > 0 {
		handler.fingerprint = fingerprinters[0]
	}
	return handler
}

// MountRoutes mounts all account mutation endpoints; reads remain under mail.read.
func (h *AccountMutationHandler) MountRoutes(mux *http.ServeMux, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("POST /v1/accounts/{accountID}/oauth", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		var req OAuthCredentialMutation
		if !h.decode(w, r, &req) {
			return
		}
		defer req.ClearSecrets()
		operations, ok := h.operations.(AccountOAuthMutationOperations)
		if !ok {
			h.reader.writeError(w, r, http.StatusServiceUnavailable, CodeInternal, "OAuth credential management is unavailable", false)
			return
		}
		accountID := r.PathValue("accountID")
		request := struct {
			AccountID string                  `json:"accountId"`
			Mutation  OAuthCredentialMutation `json:"mutation"`
		}{accountID, req}
		defer request.Mutation.ClearSecrets()
		h.mutate(w, r, request, func() (any, int, ErrorCode, string, error) {
			err := operations.SetAccountOAuthCredentials(accountID, req)
			return struct {
				Accepted bool `json:"accepted"`
			}{true}, http.StatusOK, CodeInternal, "Could not save OAuth credentials", err
		})
	})

	mux.HandleFunc("POST /v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		var req AccountMutation
		if !h.decode(w, r, &req) {
			return
		}
		defer req.Credentials.ClearSecrets()
		request := req
		defer request.Credentials.ClearSecrets()
		h.mutate(w, r, request, func() (any, int, ErrorCode, string, error) {
			acc, err := h.operations.CreateAccount(req, passwordMutation(req.Credentials))
			return accountSummaryFrom(acc), http.StatusCreated, CodeInternal, "Could not create account", err
		})
	})
	mux.HandleFunc("PUT /v1/accounts/{accountID}", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		var req AccountMutation
		if !h.decode(w, r, &req) {
			return
		}
		defer req.Credentials.ClearSecrets()
		request := req
		defer request.Credentials.ClearSecrets()
		accountID := r.PathValue("accountID")
		mutationRequest := struct {
			AccountID string          `json:"accountId"`
			Mutation  AccountMutation `json:"mutation"`
		}{accountID, request}
		defer mutationRequest.Mutation.Credentials.ClearSecrets()
		h.mutate(w, r, mutationRequest, func() (any, int, ErrorCode, string, error) {
			acc, err := h.operations.UpdateAccount(accountID, req, passwordMutation(req.Credentials))
			return accountSummaryFrom(acc), http.StatusOK, CodeInternal, "Could not update account", err
		})
	})
	mux.HandleFunc("DELETE /v1/accounts/{accountID}", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		accountID := r.PathValue("accountID")
		h.mutate(w, r, struct {
			AccountID string `json:"accountId"`
		}{accountID}, func() (any, int, ErrorCode, string, error) {
			err := h.operations.DeleteAccount(accountID)
			return struct {
				Deleted bool `json:"deleted"`
			}{true}, http.StatusOK, CodeInternal, "Could not remove account", err
		})
	})
	mux.HandleFunc("PATCH /v1/accounts/{accountID}/enabled", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		var req AccountEnabledMutation
		if !h.decode(w, r, &req) {
			return
		}
		accountID := r.PathValue("accountID")
		h.mutate(w, r, struct {
			AccountID string                 `json:"accountId"`
			Mutation  AccountEnabledMutation `json:"mutation"`
		}{accountID, req}, func() (any, int, ErrorCode, string, error) {
			acc, err := h.operations.SetAccountEnabled(accountID, req.Enabled)
			return accountSummaryFrom(acc), http.StatusOK, CodeInternal, "Could not update account status", err
		})
	})
	mux.HandleFunc("PUT /v1/accounts/order", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		var req AccountOrderMutation
		if !h.decode(w, r, &req) {
			return
		}
		h.mutate(w, r, req, func() (any, int, ErrorCode, string, error) {
			err := h.operations.ReorderAccounts(req.IDs)
			return struct {
				IDs []string `json:"ids"`
			}{req.IDs}, http.StatusOK, CodeInternal, "Could not reorder accounts", err
		})
	})
	mux.HandleFunc("POST /v1/accounts/shared-mailboxes", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityAccountManage) {
			return
		}
		var req SharedMailboxMutation
		if !h.decode(w, r, &req) {
			return
		}
		h.mutate(w, r, req, func() (any, int, ErrorCode, string, error) {
			acc, err := h.operations.AddSharedMailbox(r.Context(), req)
			return accountSummaryFrom(acc), http.StatusCreated, CodeInternal, "Could not add shared mailbox", err
		})
	})
}

func passwordMutation(mutation *PasswordCredentialMutation) PasswordCredentialMutation {
	if mutation == nil {
		return PasswordCredentialMutation{IMAPAction: PasswordKeep, SMTPAction: PasswordKeep}
	}
	return *mutation
}

func (h *AccountMutationHandler) decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		h.reader.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed or unsupported account request", false)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		h.reader.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Account request must contain one JSON value", false)
		return false
	}
	return true
}

func (h *AccountMutationHandler) mutate(w http.ResponseWriter, r *http.Request, request any, operation func() (any, int, ErrorCode, string, error)) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		h.reader.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid Idempotency-Key header is required", false)
		return
	}
	encodedRequest, err := json.Marshal(struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   any    `json:"body"`
	}{r.Method, r.URL.Path, request})
	if err != nil {
		h.reader.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed account request", false)
		return
	}
	defer clear(encodedRequest)
	var hash string
	if h.fingerprint != nil {
		fingerprint := h.fingerprint(encodedRequest)
		if len(fingerprint) == 0 {
			h.reader.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Secure account operation fingerprinting failed", false)
			return
		}
		hash = hex.EncodeToString(fingerprint)
	} else {
		if bytes.Contains(encodedRequest, []byte(`"imapPassword":`)) || bytes.Contains(encodedRequest, []byte(`"smtpPassword":`)) || bytes.Contains(encodedRequest, []byte(`"accessToken":`)) || bytes.Contains(encodedRequest, []byte(`"refreshToken":`)) || bytes.Contains(encodedRequest, []byte(`"clientSecret":`)) {
			h.reader.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Secure credential receipt fingerprinting is unavailable", false)
			return
		}
		hashBytes := sha256.Sum256(encodedRequest)
		hash = hex.EncodeToString(hashBytes[:])
	}
	prior, status, err := h.ledger.Reserve(r.Context(), key, hash)
	if errors.Is(err, ErrAccountMutationKeyConflict) {
		h.reader.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different account operation", false)
		return
	}
	if errors.Is(err, ErrAccountMutationInProgress) {
		h.reader.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "The matching account operation is still in progress", true)
		return
	}
	if err != nil {
		h.reader.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not reserve account operation", true)
		return
	}
	if prior != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(prior)
		return
	}

	data, responseStatus, errorCode, errorMessage, operationErr := operation()
	response := httptest.NewRecorder()
	if operationErr != nil {
		statusCode, publicCode, publicMessage := accountMutationError(operationErr, errorCode, errorMessage)
		h.reader.writeError(response, r, statusCode, publicCode, publicMessage, false)
	} else {
		h.reader.writeResponse(response, r, responseStatus, data, nil)
	}
	if err := h.ledger.Complete(r.Context(), key, hash, response.Code, response.Body.Bytes()); err != nil {
		h.reader.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Account operation completed but its receipt could not be saved", true)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(response.Code)
	_, _ = w.Write(bytes.Clone(response.Body.Bytes()))
}

func accountMutationError(err error, fallbackCode ErrorCode, fallbackMessage string) (int, ErrorCode, string) {
	switch {
	case errors.Is(err, ErrCredentialRepairRequired):
		return http.StatusInternalServerError, CodeCredentialRepairRequired, "Credential update could not be fully restored; account repair is required"
	case errors.Is(err, account.ErrAccountNotFound):
		return http.StatusNotFound, CodeNotFound, "Account not found"
	case errors.Is(err, account.ErrAccountExists):
		return http.StatusConflict, CodeConflict, "An account with this email already exists"
	case faultCode(err) != "":
		var fault *Fault
		errors.As(err, &fault)
		return http.StatusBadRequest, fault.Code, fault.Message
	case errors.Is(err, account.ErrNameRequired), errors.Is(err, account.ErrDisplayNameRequired),
		errors.Is(err, account.ErrEmailRequired), errors.Is(err, account.ErrIMAPHostRequired),
		errors.Is(err, account.ErrSMTPHostRequired), errors.Is(err, account.ErrUsernameRequired):
		return http.StatusBadRequest, CodeInvalidRequest, "Account configuration is incomplete"
	default:
		if fallbackCode == CodeInternal || fallbackCode == "" {
			return http.StatusInternalServerError, CodeInternal, fallbackMessage
		}
		return http.StatusInternalServerError, fallbackCode, fallbackMessage
	}
}

func faultCode(err error) ErrorCode {
	var fault *Fault
	if errors.As(err, &fault) {
		return fault.Code
	}
	return ""
}
