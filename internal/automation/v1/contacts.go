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

	"github.com/hkdb/aerion/internal/carddav"
	"github.com/hkdb/aerion/internal/contact"
	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
)

// Input bounds for the remote contacts surface. The desktop UI applies the
// same or tighter limits locally; these keep a remote client from pushing
// oversized payloads into the helper's stores.
const (
	// ContactsMaxQueryLen bounds free-text contact queries.
	ContactsMaxQueryLen = 512
	// ContactsMaxSearchLimit bounds legacy/autocomplete search results.
	ContactsMaxSearchLimit = 100
	// ContactsMaxListLimit bounds browse and legacy list page sizes.
	ContactsMaxListLimit = 500
	// ContactsMaxPhotoEmails bounds one photo batch request.
	ContactsMaxPhotoEmails = 200
	// ContactsMaxEmailLen bounds one email address in a photo batch.
	ContactsMaxEmailLen = 320
	// ContactsMaxURLLen bounds discovery URLs.
	ContactsMaxURLLen = 2048
	// contactsMaxBodyBytes bounds request bodies (inline photo data travels
	// inside create/update patches).
	contactsMaxBodyBytes = 4 << 20
)

// ErrContactNotFound marks a missing contact, source or addressbook. Mapped to
// 404 not_found on the wire.
var ErrContactNotFound = errors.New("contact not found")

// ContactSearchResult is the wire shape for legacy recipient contacts
// (autocomplete, collected addresses). ID mirrors the desktop API, which
// identifies legacy contacts by email.
type ContactSearchResult struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Source      string    `json:"source"`
	Kind        string    `json:"kind,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	SendCount   int       `json:"send_count"`
	LastUsed    time.Time `json:"last_used"`
	CreatedAt   time.Time `json:"created_at"`
}

// ContactsSourceInfo describes one configured contact source. It aliases the
// internal source record; it never carries credentials.
type ContactsSourceInfo = carddav.Source

// ContactsAddressbookInfo is one addressbook on the unified contacts surface.
type ContactsAddressbookInfo = coreapi.Addressbook

// ContactsSourceAddressbook is one synced addressbook of a CardDAV source.
type ContactsSourceAddressbook = carddav.Addressbook

// ContactsDiscoveredAddressbook is a server-side addressbook found by
// discovery, before it is enabled.
type ContactsDiscoveredAddressbook = carddav.AddressbookInfo

// ContactsStats mirrors the desktop GetContactSourceStats map keys.
type ContactsStats struct {
	TotalSources      int `json:"total_sources"`
	TotalContacts     int `json:"total_contacts"`
	SourcesWithErrors int `json:"sources_with_errors"`
}

// ContactsLinkedAccount is one email account a contact source can link to.
type ContactsLinkedAccount struct {
	AccountID       string `json:"accountId"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	IsLinked        bool   `json:"isLinked"`
	ContactSourceID string `json:"contactSourceId,omitempty"`
}

// ContactsTokenSet carries OAuth tokens for a contact source. ExpiresIn is
// seconds from receipt; the helper computes the absolute expiry.
type ContactsTokenSet struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
}

// Requests.

type ContactsSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type ContactsListLegacyRequest struct {
	Limit int `json:"limit,omitempty"`
}

type ContactsGetLegacyRequest struct {
	ID string `json:"id"`
}

type ContactsPhotosRequest struct {
	Emails []string `json:"emails"`
}

type ContactsBrowseRequest struct {
	Query    string `json:"query,omitempty"`
	SourceID string `json:"sourceId,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

type ContactsDetailRequest struct {
	ID string `json:"id"`
}

type ContactsAddressbooksRequest struct {
	SourceID string `json:"sourceId"`
}

type ContactsLinkedAccountsRequest struct {
	Kind string `json:"kind"` // "google" or "custom-oauth"
}

// ContactsMutationRequest is the single idempotent contact mutation envelope.
type ContactsMutationRequest struct {
	Operation string `json:"operation"` // add_legacy | delete_legacy | create | update | delete | set_writable | set_addressbook_enabled | sync | sync_all | force_sync

	// add_legacy
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`

	// delete_legacy, update, delete, set_writable, sync, force_sync
	ID string `json:"id,omitempty"`

	// create / update
	Create *coreapi.ContactCreateInput `json:"create,omitempty"`
	Patch  *coreapi.ContactPatch       `json:"patch,omitempty"`

	// update conflict check: RFC3339 timestamp the caller last saw
	ExpectedUpdatedAt string `json:"expectedUpdatedAt,omitempty"`

	// set_writable
	Writable bool `json:"writable,omitempty"`

	// set_addressbook_enabled
	AddressbookID string `json:"addressbookId,omitempty"`
	Enabled       bool   `json:"enabled,omitempty"`
}

// ContactsMutationResponse carries the created contact ID for create.
type ContactsMutationResponse struct {
	ID string `json:"id,omitempty"`
}

// ContactsDiscoverRequest drives server discovery and connection tests.
type ContactsDiscoverRequest struct {
	URL       string `json:"url"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	AccountID string `json:"accountId,omitempty"` // discover-oauth
}

// ContactsSourceMutationRequest creates, updates or deletes a contact source.
type ContactsSourceMutationRequest struct {
	Operation string                `json:"operation"` // create | update | delete
	ID        string                `json:"id,omitempty"`
	Config    *carddav.SourceConfig `json:"config,omitempty"`
}

// ContactsAuthorizationRequest persists the result of an OAuth consent flow
// for a contact source (the flow itself runs on the desktop).
type ContactsAuthorizationRequest struct {
	SourceID     string           `json:"sourceId,omitempty"`
	AccountID    string           `json:"accountId,omitempty"`
	Name         string           `json:"name"`
	SyncInterval int              `json:"syncInterval"`
	Provider     string           `json:"provider"`
	Email        string           `json:"email"` // verified identity from the OAuth callback
	Tokens       ContactsTokenSet `json:"tokens"`
}

// ContactsSourceTokensRequest stores refreshed or newly consented tokens for
// an existing source, keyed either by source or by account + client config.
type ContactsSourceTokensRequest struct {
	SourceID       string           `json:"sourceId,omitempty"`
	AccountID      string           `json:"accountId,omitempty"`
	ClientConfigID string           `json:"clientConfigId,omitempty"` // required with AccountID
	Provider       string           `json:"provider"`
	Scopes         []string         `json:"scopes,omitempty"`
	Tokens         ContactsTokenSet `json:"tokens"`
}

// ContactsMutationLedger records idempotent mutation outcomes. Keys are
// namespaced per surface by the implementation.
type ContactsMutationLedger interface {
	Reserve(ctx context.Context, key, hash string) ([]byte, int, error)
	Complete(ctx context.Context, key, hash string, status int, response []byte) error
}

// ContactsOperations backs the remote contacts routes. Implemented by the
// always-on helper runtime.
type ContactsOperations interface {
	SearchContacts(query string, limit int) ([]ContactSearchResult, error)
	ListLegacyContacts(limit int) ([]ContactSearchResult, error)
	GetLegacyContact(id string) (*ContactSearchResult, error)
	GetContactPhotos(emails []string) ([]contact.ContactPhoto, error)
	GetAccountProfilePhotos(emails []string) ([]contact.ContactPhoto, error)
	BrowseContacts(query, sourceID string, limit, offset int) ([]coreapi.Contact, error)
	GetContactDetail(id string) (*coreapi.Contact, error)
	ListContactSources() ([]ContactsSourceInfo, error)
	GetContactStats() (*ContactsStats, error)
	ListCoreAddressbooks(sourceID string) ([]ContactsAddressbookInfo, error)
	ListSourceAddressbooks(sourceID string) ([]ContactsSourceAddressbook, error)
	ListLinkedAccounts(kind string) ([]ContactsLinkedAccount, error)

	MutateContact(req ContactsMutationRequest) (*ContactsMutationResponse, error)

	DiscoverAddressbooks(req ContactsDiscoverRequest) ([]ContactsDiscoveredAddressbook, error)
	TestConnection(req ContactsDiscoverRequest) error
	MutateSource(req ContactsSourceMutationRequest) (*ContactsSourceInfo, error)
	SaveSourceAuthorization(req ContactsAuthorizationRequest) (*ContactsSourceInfo, error)
	StoreSourceTokens(req ContactsSourceTokensRequest) error
}

type contactsRouteHandler struct {
	ops         ContactsOperations
	ledger      ContactsMutationLedger
	fingerprint func([]byte) []byte
}

// MountContactsRoutes mounts the capability-gated remote contacts surface.
// requireCap mirrors automation.RequireGrantCapability. ledger may be nil, in
// which case mutation routes reject requests (mutations are never run without
// idempotency protection).
func MountContactsRoutes(mux *http.ServeMux, ops ContactsOperations, ledger ContactsMutationLedger, fingerprint func([]byte) []byte, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	h := &contactsRouteHandler{ops: ops, ledger: ledger, fingerprint: fingerprint}

	read := func(fn http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !requireCap(w, r, CapabilityContactsRead) {
				return
			}
			fn(w, r)
		}
	}
	write := func(fn http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !requireCap(w, r, CapabilityContactsWrite) {
				return
			}
			fn(w, r)
		}
	}
	manage := func(fn http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !requireCap(w, r, CapabilityContactsManage) {
				return
			}
			fn(w, r)
		}
	}

	mux.HandleFunc("POST /v1/contacts/search", read(h.search))
	mux.HandleFunc("POST /v1/contacts/legacy/list", read(h.listLegacy))
	mux.HandleFunc("POST /v1/contacts/legacy/get", read(h.getLegacy))
	mux.HandleFunc("POST /v1/contacts/photos", read(h.photos))
	mux.HandleFunc("POST /v1/contacts/account-photos", read(h.accountPhotos))
	mux.HandleFunc("POST /v1/contacts/browse", read(h.browse))
	mux.HandleFunc("POST /v1/contacts/detail", read(h.detail))
	mux.HandleFunc("GET /v1/contacts/sources", read(h.sources))
	mux.HandleFunc("GET /v1/contacts/stats", read(h.stats))
	mux.HandleFunc("POST /v1/contacts/addressbooks", read(h.addressbooks))
	mux.HandleFunc("POST /v1/contacts/source-addressbooks", read(h.sourceAddressbooks))
	mux.HandleFunc("POST /v1/contacts/linked-accounts", read(h.linkedAccounts))

	mux.HandleFunc("POST /v1/contacts/mutations", write(h.mutateContact))

	mux.HandleFunc("POST /v1/contacts/sources/discover", manage(h.discover))
	mux.HandleFunc("POST /v1/contacts/sources/test", manage(h.testConnection))
	mux.HandleFunc("POST /v1/contacts/sources/discover-oauth", manage(h.discoverOAuth))
	mux.HandleFunc("POST /v1/contacts/sources/mutations", manage(h.mutateSource))
	mux.HandleFunc("POST /v1/contacts/sources/authorization", manage(h.saveAuthorization))
	mux.HandleFunc("POST /v1/contacts/sources/tokens", manage(h.storeTokens))
}

func writeContactsResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeAPIResponse(w, r, status, data)
}

func writeContactsError(w http.ResponseWriter, r *http.Request, status int, code ErrorCode, message string, retryable bool) {
	writeHeaderError(w, r, status, code, message, retryable)
}

// writeContactsFault maps operation errors onto the stable error envelope.
// Unknown errors never leak internals.
func writeContactsFault(w http.ResponseWriter, r *http.Request, err error) {
	var conflict *coreapi.ErrConflict
	var consent *coreapi.ErrAdditionalConsentRequired
	var fault *Fault
	switch {
	case errors.As(err, &fault):
		status := http.StatusInternalServerError
		switch fault.Code {
		case CodeInvalidRequest:
			status = http.StatusBadRequest
		case CodeNotFound:
			status = http.StatusNotFound
		case CodeConflict, CodeRevisionConflict, CodeIdempotencyConflict:
			status = http.StatusConflict
		case CodeProviderError, CodeCapabilityDenied:
			status = http.StatusForbidden
		case CodeAccountUnreachable:
			status = http.StatusBadGateway
		case CodeDatabaseBusy:
			status = http.StatusServiceUnavailable
		}
		writeContactsError(w, r, status, fault.Code, fault.Message, fault.Retryable)
	case errors.As(err, &conflict):
		code := CodeConflict
		if strings.Contains(strings.ToLower(conflict.Message), "expected revision") {
			code = CodeRevisionConflict
		}
		writeContactsError(w, r, http.StatusConflict, code, conflict.Message, false)
	case errors.As(err, &consent):
		writeContactsError(w, r, http.StatusForbidden, CodeProviderError, "The contact provider requires additional consent: "+consent.Error(), false)
	case errors.Is(err, ErrContactNotFound):
		writeContactsError(w, r, http.StatusNotFound, CodeNotFound, "Contact, source or addressbook not found", false)
	case errors.Is(err, coreapi.ErrUnimplemented):
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "This contact operation is not available on the remote helper", false)
	default:
		writeContactsError(w, r, http.StatusInternalServerError, CodeInternal, "The contact operation failed", true)
	}
}

func (h *contactsRouteHandler) decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, contactsMaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed or unsupported contacts request", false)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Contacts request must contain one JSON value", false)
		return false
	}
	return true
}

func clampLimit(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func validDiscoveryURL(rawURL string) bool {
	if len(rawURL) == 0 || len(rawURL) > ContactsMaxURLLen {
		return false
	}
	return strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://")
}

func (h *contactsRouteHandler) search(w http.ResponseWriter, r *http.Request) {
	var req ContactsSearchRequest
	if !h.decode(w, r, &req) {
		return
	}
	if len(req.Query) > ContactsMaxQueryLen {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Query is too long", false)
		return
	}
	results, err := h.ops.SearchContacts(req.Query, clampLimit(req.Limit, 50, ContactsMaxSearchLimit))
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, results)
}

func (h *contactsRouteHandler) listLegacy(w http.ResponseWriter, r *http.Request) {
	var req ContactsListLegacyRequest
	if !h.decode(w, r, &req) {
		return
	}
	results, err := h.ops.ListLegacyContacts(clampLimit(req.Limit, 100, ContactsMaxListLimit))
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, results)
}

func (h *contactsRouteHandler) getLegacy(w http.ResponseWriter, r *http.Request) {
	var req ContactsGetLegacyRequest
	if !h.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A contact ID is required", false)
		return
	}
	result, err := h.ops.GetLegacyContact(req.ID)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, result)
}

func (h *contactsRouteHandler) photos(w http.ResponseWriter, r *http.Request) {
	h.photoBatch(w, r, h.ops.GetContactPhotos)
}

func (h *contactsRouteHandler) accountPhotos(w http.ResponseWriter, r *http.Request) {
	h.photoBatch(w, r, h.ops.GetAccountProfilePhotos)
}

func (h *contactsRouteHandler) photoBatch(w http.ResponseWriter, r *http.Request, op func([]string) ([]contact.ContactPhoto, error)) {
	var req ContactsPhotosRequest
	if !h.decode(w, r, &req) {
		return
	}
	if len(req.Emails) > ContactsMaxPhotoEmails {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Too many addresses in one photo request", false)
		return
	}
	for _, email := range req.Emails {
		if len(email) > ContactsMaxEmailLen {
			writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "An address in the photo request is too long", false)
			return
		}
	}
	photos, err := op(req.Emails)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, photos)
}

func (h *contactsRouteHandler) browse(w http.ResponseWriter, r *http.Request) {
	var req ContactsBrowseRequest
	if !h.decode(w, r, &req) {
		return
	}
	if len(req.Query) > ContactsMaxQueryLen {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Query is too long", false)
		return
	}
	contacts, err := h.ops.BrowseContacts(req.Query, req.SourceID, clampLimit(req.Limit, 200, ContactsMaxListLimit), max(req.Offset, 0))
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, contacts)
}

func (h *contactsRouteHandler) detail(w http.ResponseWriter, r *http.Request) {
	var req ContactsDetailRequest
	if !h.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A contact ID or email is required", false)
		return
	}
	detail, err := h.ops.GetContactDetail(req.ID)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, detail)
}

func (h *contactsRouteHandler) sources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.ops.ListContactSources()
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, sources)
}

func (h *contactsRouteHandler) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.ops.GetContactStats()
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, stats)
}

func (h *contactsRouteHandler) addressbooks(w http.ResponseWriter, r *http.Request) {
	var req ContactsAddressbooksRequest
	if !h.decode(w, r, &req) {
		return
	}
	books, err := h.ops.ListCoreAddressbooks(req.SourceID)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, books)
}

func (h *contactsRouteHandler) sourceAddressbooks(w http.ResponseWriter, r *http.Request) {
	var req ContactsAddressbooksRequest
	if !h.decode(w, r, &req) {
		return
	}
	books, err := h.ops.ListSourceAddressbooks(req.SourceID)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, books)
}

func (h *contactsRouteHandler) linkedAccounts(w http.ResponseWriter, r *http.Request) {
	var req ContactsLinkedAccountsRequest
	if !h.decode(w, r, &req) {
		return
	}
	if req.Kind != "google" && req.Kind != "custom-oauth" {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Kind must be \"google\" or \"custom-oauth\"", false)
		return
	}
	accounts, err := h.ops.ListLinkedAccounts(req.Kind)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, accounts)
}

func (h *contactsRouteHandler) mutateContact(w http.ResponseWriter, r *http.Request) {
	var req ContactsMutationRequest
	if !h.decode(w, r, &req) {
		return
	}
	if !validContactMutation(req.Operation) {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Unknown contact mutation operation", false)
		return
	}
	h.mutate(w, r, &req, func() (any, int, error) {
		resp, err := h.ops.MutateContact(req)
		if err != nil {
			return nil, 0, err
		}
		return resp, http.StatusOK, nil
	})
}

func validContactMutation(operation string) bool {
	switch operation {
	case "add_legacy", "delete_legacy", "create", "update", "delete",
		"set_writable", "set_addressbook_enabled", "sync", "sync_all", "force_sync":
		return true
	}
	return false
}

func (h *contactsRouteHandler) discover(w http.ResponseWriter, r *http.Request) {
	var req ContactsDiscoverRequest
	if !h.decode(w, r, &req) {
		return
	}
	if !validDiscoveryURL(req.URL) {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid http(s) discovery URL is required", false)
		return
	}
	books, err := h.ops.DiscoverAddressbooks(req)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, books)
}

func (h *contactsRouteHandler) testConnection(w http.ResponseWriter, r *http.Request) {
	var req ContactsDiscoverRequest
	if !h.decode(w, r, &req) {
		return
	}
	if !validDiscoveryURL(req.URL) {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid http(s) discovery URL is required", false)
		return
	}
	if err := h.ops.TestConnection(req); err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, struct{}{})
}

func (h *contactsRouteHandler) discoverOAuth(w http.ResponseWriter, r *http.Request) {
	var req ContactsDiscoverRequest
	if !h.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.AccountID) == "" {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "An account ID is required", false)
		return
	}
	books, err := h.ops.DiscoverAddressbooks(req)
	if err != nil {
		writeContactsFault(w, r, err)
		return
	}
	writeContactsResponse(w, r, http.StatusOK, books)
}

func (h *contactsRouteHandler) mutateSource(w http.ResponseWriter, r *http.Request) {
	var req ContactsSourceMutationRequest
	if !h.decode(w, r, &req) {
		return
	}
	switch req.Operation {
	case "create", "update":
		if req.Config == nil {
			writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A source config is required", false)
			return
		}
	case "delete":
		if strings.TrimSpace(req.ID) == "" {
			writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A source ID is required", false)
			return
		}
	default:
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Unknown source mutation operation", false)
		return
	}
	h.mutate(w, r, &req, func() (any, int, error) {
		source, err := h.ops.MutateSource(req)
		if err != nil {
			return nil, 0, err
		}
		status := http.StatusOK
		if req.Operation == "create" {
			status = http.StatusCreated
		}
		return source, status, nil
	})
}

func (h *contactsRouteHandler) saveAuthorization(w http.ResponseWriter, r *http.Request) {
	var req ContactsAuthorizationRequest
	if !h.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Provider) == "" ||
		strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Tokens.AccessToken) == "" {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Name, provider, verified email and tokens are required", false)
		return
	}
	h.mutate(w, r, &req, func() (any, int, error) {
		source, err := h.ops.SaveSourceAuthorization(req)
		if err != nil {
			return nil, 0, err
		}
		return source, http.StatusCreated, nil
	})
}

func (h *contactsRouteHandler) storeTokens(w http.ResponseWriter, r *http.Request) {
	var req ContactsSourceTokensRequest
	if !h.decode(w, r, &req) {
		return
	}
	sourceKeyed := strings.TrimSpace(req.SourceID) != ""
	accountKeyed := strings.TrimSpace(req.AccountID) != "" && strings.TrimSpace(req.ClientConfigID) != ""
	if sourceKeyed == accountKeyed {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Exactly one of sourceId or accountId+clientConfigId is required", false)
		return
	}
	if strings.TrimSpace(req.Tokens.AccessToken) == "" || strings.TrimSpace(req.Provider) == "" {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Provider and tokens are required", false)
		return
	}
	h.mutate(w, r, &req, func() (any, int, error) {
		if err := h.ops.StoreSourceTokens(req); err != nil {
			return nil, 0, err
		}
		return struct{}{}, http.StatusOK, nil
	})
}

// mutate runs one idempotent mutation: it reserves the Idempotency-Key,
// replays a stored outcome when the key was already completed, and records the
// outcome (success or failure) so a retry never re-executes.
func (h *contactsRouteHandler) mutate(w http.ResponseWriter, r *http.Request, request any, operation func() (any, int, error)) {
	if h.ledger == nil {
		writeContactsError(w, r, http.StatusServiceUnavailable, CodeInternal, "Contact mutations are unavailable on this helper", true)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid Idempotency-Key header is required", false)
		return
	}
	encodedRequest, err := json.Marshal(struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   any    `json:"body"`
	}{r.Method, r.URL.Path, request})
	if err != nil {
		writeContactsError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed contacts request", false)
		return
	}
	defer clear(encodedRequest)
	var hash string
	if h.fingerprint != nil {
		fingerprint := h.fingerprint(encodedRequest)
		if len(fingerprint) == 0 {
			writeContactsError(w, r, http.StatusInternalServerError, CodeInternal, "Secure contact operation fingerprinting failed", false)
			return
		}
		hash = hex.EncodeToString(fingerprint)
	} else {
		// Without a keyed fingerprint, refuse to hash bodies that carry
		// credentials: a plain hash of a low-entropy secret is recoverable.
		if bytes.Contains(encodedRequest, []byte(`"accessToken":`)) ||
			bytes.Contains(encodedRequest, []byte(`"refreshToken":`)) ||
			bytes.Contains(encodedRequest, []byte(`"password":`)) ||
			bytes.Contains(encodedRequest, []byte(`"Password":`)) {
			writeContactsError(w, r, http.StatusInternalServerError, CodeInternal, "Secure credential receipt fingerprinting is unavailable", false)
			return
		}
		hashBytes := sha256.Sum256(encodedRequest)
		hash = hex.EncodeToString(hashBytes[:])
	}

	prior, status, err := h.ledger.Reserve(r.Context(), key, hash)
	if errors.Is(err, ErrAccountMutationKeyConflict) {
		writeContactsError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different contact operation", false)
		return
	}
	if errors.Is(err, ErrAccountMutationInProgress) {
		writeContactsError(w, r, http.StatusConflict, CodeIdempotencyConflict, "The matching contact operation is still in progress", true)
		return
	}
	if err != nil {
		writeContactsError(w, r, http.StatusInternalServerError, CodeInternal, "Could not reserve contact operation", true)
		return
	}
	if prior != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(prior)
		return
	}

	data, responseStatus, operationErr := operation()
	response := httptest.NewRecorder()
	if operationErr != nil {
		writeContactsFault(response, r, operationErr)
	} else {
		writeContactsResponse(response, r, responseStatus, data)
	}
	if err := h.ledger.Complete(r.Context(), key, hash, response.Code, response.Body.Bytes()); err != nil {
		writeContactsError(w, r, http.StatusInternalServerError, CodeInternal, "Contact operation completed but its receipt could not be saved", true)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(response.Code)
	_, _ = w.Write(response.Body.Bytes())
}
