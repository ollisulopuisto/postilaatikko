package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hkdb/aerion/internal/contact"
	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
)

// contactsFakeOps implements ContactsOperations with overridable funcs.
type contactsFakeOps struct {
	mu          sync.Mutex
	mutateCalls int
	mutateErr   error
	mutateResp  *ContactsMutationResponse
}

func (f *contactsFakeOps) SearchContacts(query string, limit int) ([]ContactSearchResult, error) {
	return []ContactSearchResult{}, nil
}

func (f *contactsFakeOps) ListLegacyContacts(limit int) ([]ContactSearchResult, error) {
	return []ContactSearchResult{}, nil
}

func (f *contactsFakeOps) GetLegacyContact(id string) (*ContactSearchResult, error) {
	return &ContactSearchResult{ID: id}, nil
}

func (f *contactsFakeOps) GetContactPhotos(emails []string) ([]contact.ContactPhoto, error) {
	return []contact.ContactPhoto{}, nil
}

func (f *contactsFakeOps) GetAccountProfilePhotos(emails []string) ([]contact.ContactPhoto, error) {
	return []contact.ContactPhoto{}, nil
}

func (f *contactsFakeOps) BrowseContacts(query, sourceID string, limit, offset int) ([]coreapi.Contact, error) {
	return []coreapi.Contact{}, nil
}

func (f *contactsFakeOps) GetContactDetail(id string) (*coreapi.Contact, error) {
	return &coreapi.Contact{ID: id}, nil
}

func (f *contactsFakeOps) ListContactSources() ([]ContactsSourceInfo, error) {
	return []ContactsSourceInfo{}, nil
}

func (f *contactsFakeOps) GetContactStats() (*ContactsStats, error) {
	return &ContactsStats{}, nil
}

func (f *contactsFakeOps) ListCoreAddressbooks(sourceID string) ([]ContactsAddressbookInfo, error) {
	return []ContactsAddressbookInfo{}, nil
}

func (f *contactsFakeOps) ListSourceAddressbooks(sourceID string) ([]ContactsSourceAddressbook, error) {
	return []ContactsSourceAddressbook{}, nil
}

func (f *contactsFakeOps) ListLinkedAccounts(kind string) ([]ContactsLinkedAccount, error) {
	return []ContactsLinkedAccount{}, nil
}

func (f *contactsFakeOps) MutateContact(req ContactsMutationRequest) (*ContactsMutationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mutateCalls++
	if f.mutateErr != nil {
		return nil, f.mutateErr
	}
	if f.mutateResp != nil {
		return f.mutateResp, nil
	}
	return &ContactsMutationResponse{}, nil
}

func (f *contactsFakeOps) DiscoverAddressbooks(req ContactsDiscoverRequest) ([]ContactsDiscoveredAddressbook, error) {
	return []ContactsDiscoveredAddressbook{}, nil
}

func (f *contactsFakeOps) TestConnection(req ContactsDiscoverRequest) error {
	return nil
}

func (f *contactsFakeOps) MutateSource(req ContactsSourceMutationRequest) (*ContactsSourceInfo, error) {
	return &ContactsSourceInfo{ID: "src-1", Name: "Test"}, nil
}

func (f *contactsFakeOps) SaveSourceAuthorization(req ContactsAuthorizationRequest) (*ContactsSourceInfo, error) {
	return &ContactsSourceInfo{ID: "src-1", Name: "Test"}, nil
}

func (f *contactsFakeOps) StoreSourceTokens(req ContactsSourceTokensRequest) error {
	return nil
}

// contactsFakeLedger is an in-memory ContactsMutationLedger.
type contactsFakeLedger struct {
	mu        sync.Mutex
	responses map[string][]byte
	status    map[string]int
	hashes    map[string]string
	pending   bool
}

func newContactsFakeLedger() *contactsFakeLedger {
	return &contactsFakeLedger{
		responses: map[string][]byte{},
		status:    map[string]int{},
		hashes:    map[string]string{},
	}
}

func (l *contactsFakeLedger) Reserve(_ context.Context, key, hash string) ([]byte, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending {
		return nil, 0, ErrAccountMutationInProgress
	}
	if prior, ok := l.hashes[key]; ok {
		if prior != hash {
			return nil, 0, ErrAccountMutationKeyConflict
		}
		if resp, ok := l.responses[key]; ok {
			return resp, l.status[key], nil
		}
		return nil, 0, ErrAccountMutationInProgress
	}
	l.hashes[key] = hash
	return nil, 0, nil
}

func (l *contactsFakeLedger) Complete(_ context.Context, key, hash string, status int, response []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if prior, ok := l.hashes[key]; ok && prior != hash {
		return ErrAccountMutationKeyConflict
	}
	l.hashes[key] = hash
	l.responses[key] = append([]byte(nil), response...)
	l.status[key] = status
	return nil
}

// contactsTestServer builds a mux with the contacts routes mounted and a
// stub capability checker driven by the X-Test-Capabilities header.
func contactsTestServer(ops ContactsOperations, ledger ContactsMutationLedger) *httptest.Server {
	mux := http.NewServeMux()
	requireCap := func(w http.ResponseWriter, r *http.Request, capability Capability) bool {
		granted := strings.Split(r.Header.Get("X-Test-Capabilities"), ",")
		for _, g := range granted {
			if strings.TrimSpace(g) == string(capability) {
				return true
			}
		}
		writeContactsError(w, r, http.StatusForbidden, CodeCapabilityDenied, "The client grant does not allow "+string(capability), false)
		return false
	}
	MountContactsRoutes(mux, ops, ledger, nil, requireCap)
	return httptest.NewServer(mux)
}

func contactsRequest(t *testing.T, srv *httptest.Server, caps, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if caps != "" {
		req.Header.Set("X-Test-Capabilities", caps)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	return resp
}

func contactsDecodeError(t *testing.T, resp *http.Response) ErrorBody {
	t.Helper()
	defer resp.Body.Close()
	var envelope ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope (status %d): %v", resp.StatusCode, err)
	}
	return envelope.Error
}

func TestContactsRoutesRequireCapabilities(t *testing.T) {
	srv := contactsTestServer(&contactsFakeOps{}, newContactsFakeLedger())
	defer srv.Close()

	cases := []struct {
		name       string
		method     string
		path       string
		body       any
		capability Capability
	}{
		{"search", http.MethodPost, "/v1/contacts/search", map[string]any{"query": "ada"}, CapabilityContactsRead},
		{"legacy list", http.MethodPost, "/v1/contacts/legacy/list", map[string]any{"limit": 50}, CapabilityContactsRead},
		{"legacy get", http.MethodPost, "/v1/contacts/legacy/get", map[string]any{"id": "ada@example.com"}, CapabilityContactsRead},
		{"photos", http.MethodPost, "/v1/contacts/photos", map[string]any{"emails": []string{"a@example.com"}}, CapabilityContactsRead},
		{"account photos", http.MethodPost, "/v1/contacts/account-photos", map[string]any{"emails": []string{"a@example.com"}}, CapabilityContactsRead},
		{"browse", http.MethodPost, "/v1/contacts/browse", map[string]any{"limit": 50}, CapabilityContactsRead},
		{"detail", http.MethodPost, "/v1/contacts/detail", map[string]any{"id": "c-1"}, CapabilityContactsRead},
		{"sources", http.MethodGet, "/v1/contacts/sources", nil, CapabilityContactsRead},
		{"stats", http.MethodGet, "/v1/contacts/stats", nil, CapabilityContactsRead},
		{"addressbooks", http.MethodPost, "/v1/contacts/addressbooks", map[string]any{"sourceId": "src-1"}, CapabilityContactsRead},
		{"source addressbooks", http.MethodPost, "/v1/contacts/source-addressbooks", map[string]any{"sourceId": "src-1"}, CapabilityContactsRead},
		{"linked accounts", http.MethodPost, "/v1/contacts/linked-accounts", map[string]any{"kind": "google"}, CapabilityContactsRead},
		{"mutation", http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "a@example.com"}, CapabilityContactsWrite},
		{"discover", http.MethodPost, "/v1/contacts/sources/discover", map[string]any{"url": "https://dav.example.com/"}, CapabilityContactsManage},
		{"test connection", http.MethodPost, "/v1/contacts/sources/test", map[string]any{"url": "https://dav.example.com/"}, CapabilityContactsManage},
		{"discover oauth", http.MethodPost, "/v1/contacts/sources/discover-oauth", map[string]any{"accountId": "acc-1"}, CapabilityContactsManage},
		{"source mutation", http.MethodPost, "/v1/contacts/sources/mutations", map[string]any{"operation": "delete", "id": "src-1"}, CapabilityContactsManage},
		{"authorization", http.MethodPost, "/v1/contacts/sources/authorization", map[string]any{"name": "Google", "provider": "google-contacts", "email": "a@example.com", "tokens": map[string]any{"accessToken": "x", "refreshToken": "y", "expiresIn": 3600}}, CapabilityContactsManage},
		{"tokens", http.MethodPost, "/v1/contacts/sources/tokens", map[string]any{"sourceId": "src-1", "provider": "google-contacts", "tokens": map[string]any{"accessToken": "x", "refreshToken": "y", "expiresIn": 3600}}, CapabilityContactsManage},
	}

	for _, tc := range cases {
		t.Run(tc.name+" denied without grant", func(t *testing.T) {
			resp := contactsRequest(t, srv, "", tc.method, tc.path, tc.body, map[string]string{"Idempotency-Key": "k-1"})
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.StatusCode)
			}
			if errBody := contactsDecodeError(t, resp); errBody.Code != CodeCapabilityDenied {
				t.Fatalf("expected capability_denied, got %q", errBody.Code)
			}
		})
		t.Run(tc.name+" allowed with grant", func(t *testing.T) {
			resp := contactsRequest(t, srv, string(tc.capability), tc.method, tc.path, tc.body, map[string]string{"Idempotency-Key": "k-2"})
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden {
				t.Fatalf("expected non-403 with %s grant, got 403", tc.capability)
			}
		})
	}

	// A read grant must not mutate; a write grant must not administer sources.
	t.Run("read grant cannot mutate", func(t *testing.T) {
		resp := contactsRequest(t, srv, string(CapabilityContactsRead), http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "a@example.com"}, map[string]string{"Idempotency-Key": "k-3"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
		contactsDecodeError(t, resp)
	})
	t.Run("write grant cannot administer sources", func(t *testing.T) {
		resp := contactsRequest(t, srv, string(CapabilityContactsWrite), http.MethodPost, "/v1/contacts/sources/mutations", map[string]any{"operation": "delete", "id": "src-1"}, map[string]string{"Idempotency-Key": "k-4"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
		contactsDecodeError(t, resp)
	})
	// mail capabilities must not disclose contact data.
	t.Run("mail grants cannot read contacts", func(t *testing.T) {
		resp := contactsRequest(t, srv, string(CapabilityMailRead)+","+string(CapabilityMailWrite), http.MethodPost, "/v1/contacts/search", map[string]any{"query": "ada"}, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
		contactsDecodeError(t, resp)
	})
}

func TestContactsInputBounds(t *testing.T) {
	srv := contactsTestServer(&contactsFakeOps{}, newContactsFakeLedger())
	defer srv.Close()
	read := string(CapabilityContactsRead)

	t.Run("oversized query rejected", func(t *testing.T) {
		resp := contactsRequest(t, srv, read, http.MethodPost, "/v1/contacts/search", map[string]any{"query": strings.Repeat("a", ContactsMaxQueryLen+1)}, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
		if errBody := contactsDecodeError(t, resp); errBody.Code != CodeInvalidRequest {
			t.Fatalf("expected invalid_request, got %q", errBody.Code)
		}
	})

	t.Run("search limit clamped", func(t *testing.T) {
		ops := &contactsFakeOps{}
		srv2 := contactsTestServer(ops, newContactsFakeLedger())
		defer srv2.Close()
		resp := contactsRequest(t, srv2, read, http.MethodPost, "/v1/contacts/search", map[string]any{"query": "ada", "limit": 10000}, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("too many photo emails rejected", func(t *testing.T) {
		emails := make([]string, ContactsMaxPhotoEmails+1)
		for i := range emails {
			emails[i] = "a@example.com"
		}
		resp := contactsRequest(t, srv, read, http.MethodPost, "/v1/contacts/photos", map[string]any{"emails": emails}, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
		contactsDecodeError(t, resp)
	})

	t.Run("discover rejects non-http URL", func(t *testing.T) {
		manage := string(CapabilityContactsManage)
		for _, rawURL := range []string{"file:///etc/passwd", "gopher://example.com", strings.Repeat("https://", 400)} {
			resp := contactsRequest(t, srv, manage, http.MethodPost, "/v1/contacts/sources/discover", map[string]any{"url": rawURL}, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d", rawURL, resp.StatusCode)
			}
			contactsDecodeError(t, resp)
		}
	})

	t.Run("unknown mutation operation rejected", func(t *testing.T) {
		resp := contactsRequest(t, srv, string(CapabilityContactsWrite), http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "nuke"}, map[string]string{"Idempotency-Key": "k-unknown-op"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
		if errBody := contactsDecodeError(t, resp); errBody.Code != CodeInvalidRequest {
			t.Fatalf("expected invalid_request, got %q", errBody.Code)
		}
	})
}

func TestContactsMutationIdempotency(t *testing.T) {
	ops := &contactsFakeOps{}
	ledger := newContactsFakeLedger()
	srv := contactsTestServer(ops, ledger)
	defer srv.Close()
	write := string(CapabilityContactsWrite)
	body := map[string]any{"operation": "delete_legacy", "id": "a@example.com"}

	t.Run("missing idempotency key rejected", func(t *testing.T) {
		resp := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", body, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
		if errBody := contactsDecodeError(t, resp); errBody.Code != CodeInvalidRequest {
			t.Fatalf("expected invalid_request, got %q", errBody.Code)
		}
	})

	t.Run("replay returns stored response without re-executing", func(t *testing.T) {
		first := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", body, map[string]string{"Idempotency-Key": "k-replay"})
		first.Body.Close()
		if first.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", first.StatusCode)
		}
		second := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", body, map[string]string{"Idempotency-Key": "k-replay"})
		defer second.Body.Close()
		if second.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on replay, got %d", second.StatusCode)
		}
		if ops.mutateCalls != 1 {
			t.Fatalf("expected 1 underlying execution, got %d", ops.mutateCalls)
		}
	})

	t.Run("key reuse with different body conflicts", func(t *testing.T) {
		other := map[string]any{"operation": "delete_legacy", "id": "b@example.com"}
		resp := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", other, map[string]string{"Idempotency-Key": "k-replay"})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.StatusCode)
		}
		if errBody := contactsDecodeError(t, resp); errBody.Code != CodeIdempotencyConflict {
			t.Fatalf("expected idempotency_conflict, got %q", errBody.Code)
		}
	})

	t.Run("in progress reports conflict", func(t *testing.T) {
		ledger.mu.Lock()
		ledger.pending = true
		ledger.mu.Unlock()
		defer func() {
			ledger.mu.Lock()
			ledger.pending = false
			ledger.mu.Unlock()
		}()
		resp := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "c@example.com"}, map[string]string{"Idempotency-Key": "k-stuck"})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.StatusCode)
		}
		if errBody := contactsDecodeError(t, resp); errBody.Code != CodeIdempotencyConflict {
			t.Fatalf("expected idempotency_conflict, got %q", errBody.Code)
		}
	})
}

func TestContactsMutationErrorMapping(t *testing.T) {
	write := string(CapabilityContactsWrite)

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   ErrorCode
	}{
		{"provider conflict", &coreapi.ErrConflict{Message: "contact changed on server"}, http.StatusConflict, CodeConflict},
		{"revision mismatch", &coreapi.ErrConflict{Message: "expected revision does not match"}, http.StatusConflict, CodeRevisionConflict},
		{"consent required", &coreapi.ErrAdditionalConsentRequired{AccountID: "acc-1", ClientConfigID: "google", MissingScopes: []coreapi.AuthScope{{Resource: "https://www.googleapis.com/auth/contacts.readonly"}}}, http.StatusForbidden, CodeProviderError},
		{"not found", fmt.Errorf("lookup: %w", ErrContactNotFound), http.StatusNotFound, CodeNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops := &contactsFakeOps{mutateErr: tc.err}
			srv := contactsTestServer(ops, newContactsFakeLedger())
			defer srv.Close()
			resp := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "a@example.com"}, map[string]string{"Idempotency-Key": "k-" + tc.name})
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("expected %d, got %d", tc.wantStatus, resp.StatusCode)
			}
			if errBody := contactsDecodeError(t, resp); errBody.Code != tc.wantCode {
				t.Fatalf("expected %q, got %q", tc.wantCode, errBody.Code)
			}
		})
	}

	t.Run("unknown errors hide internals", func(t *testing.T) {
		ops := &contactsFakeOps{mutateErr: errors.New("sqlite: secret table layout xyz")}
		srv := contactsTestServer(ops, newContactsFakeLedger())
		defer srv.Close()
		resp := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "a@example.com"}, map[string]string{"Idempotency-Key": "k-secret"})
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", resp.StatusCode)
		}
		errBody := contactsDecodeError(t, resp)
		if errBody.Code != CodeInternal {
			t.Fatalf("expected internal_error, got %q", errBody.Code)
		}
		if strings.Contains(errBody.Message, "sqlite") || strings.Contains(errBody.Message, "secret") {
			t.Fatalf("internal error leaked: %q", errBody.Message)
		}
	})

	t.Run("mutation failure does not leave a replayable success", func(t *testing.T) {
		ops := &contactsFakeOps{mutateErr: errors.New("boom")}
		ledger := newContactsFakeLedger()
		srv := contactsTestServer(ops, ledger)
		defer srv.Close()
		first := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "a@example.com"}, map[string]string{"Idempotency-Key": "k-fail"})
		first.Body.Close()
		if first.StatusCode != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", first.StatusCode)
		}
		// The failed outcome is recorded; a retry with the same key replays the
		// failure response rather than re-executing.
		ops.mu.Lock()
		ops.mutateErr = nil
		ops.mu.Unlock()
		second := contactsRequest(t, srv, write, http.MethodPost, "/v1/contacts/mutations", map[string]any{"operation": "delete_legacy", "id": "a@example.com"}, map[string]string{"Idempotency-Key": "k-fail"})
		defer second.Body.Close()
		if second.StatusCode != http.StatusInternalServerError {
			t.Fatalf("expected replayed 500, got %d", second.StatusCode)
		}
		if ops.mutateCalls != 1 {
			t.Fatalf("expected 1 underlying execution, got %d", ops.mutateCalls)
		}
	})
}
