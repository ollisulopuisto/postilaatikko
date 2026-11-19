package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

type stubBootstrapOps struct {
	accounts        []*account.Account
	accountsErr     error
	folders         map[string][]*folder.Folder
	folderErr       map[string]error
	views           []*virtualfolder.VirtualFolder
	viewsErr        error
	counts          map[string]int
	countsErr       error
	conversations   []*message.Conversation
	conversationErr error
}

func (s *stubBootstrapOps) GetAccounts() ([]*account.Account, error) {
	return s.accounts, s.accountsErr
}
func (s *stubBootstrapOps) GetFolders(accountID string) ([]*folder.Folder, error) {
	if err := s.folderErr[accountID]; err != nil {
		return nil, err
	}
	return s.folders[accountID], nil
}
func (s *stubBootstrapOps) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) {
	return s.views, s.viewsErr
}
func (s *stubBootstrapOps) GetVirtualFolderUnreadCounts() (map[string]int, error) {
	return s.counts, s.countsErr
}
func (s *stubBootstrapOps) GetFolder(folderID string) (*folder.Folder, error) {
	for _, folders := range s.folders {
		for _, f := range folders {
			if f.ID == folderID {
				return f, nil
			}
		}
	}
	return nil, nil
}
func (s *stubBootstrapOps) ListConversationsByFolder(folderID string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	return s.conversations, s.conversationErr
}
func (s *stubBootstrapOps) ListConversationsUnifiedInbox(offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	return s.conversations, s.conversationErr
}
func (s *stubBootstrapOps) ListConversationsUnifiedFolder(folderType string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	return s.conversations, s.conversationErr
}

func newBootstrapServer(ops WorkspaceBootstrapOperations, allow bool) http.Handler {
	mux := http.NewServeMux()
	MountWorkspaceBootstrapRoute(mux, ops, func(w http.ResponseWriter, r *http.Request, c Capability) bool {
		if !allow {
			w.WriteHeader(http.StatusForbidden)
			return false
		}
		return true
	})
	return mux
}

func bootstrapRequest(t *testing.T, server http.Handler, body string) (int, WorkspaceBootstrapResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/workspace/bootstrap", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	var envelope Response[WorkspaceBootstrapResponse]
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, envelope.Data
}

func goodBootstrapOps() *stubBootstrapOps {
	return &stubBootstrapOps{
		accounts: []*account.Account{{ID: "acc1", Email: "a@example.com"}},
		folders: map[string][]*folder.Folder{
			"acc1": {{ID: "f1", AccountID: "acc1", Name: "Inbox"}},
		},
		views:  []*virtualfolder.VirtualFolder{{ID: "vf1", Name: "Reply later"}},
		counts: map[string]int{"vf1": 3},
		conversations: []*message.Conversation{
			{ThreadID: "c1", Subject: "hello", Messages: []*message.Message{{ID: "m1", BodyText: "secret"}}},
		},
	}
}

const bootstrapBody = `{"initialPage":{"scope":{"kind":"folder","accountId":"acc1","folderId":"f1"},"page":{"limit":10}}}`

func TestWorkspaceBootstrapAggregatesAllSections(t *testing.T) {
	code, resp := bootstrapRequest(t, newBootstrapServer(goodBootstrapOps(), true), bootstrapBody)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(resp.Accounts) != 1 || resp.Accounts[0].ID != "acc1" {
		t.Fatalf("accounts = %+v", resp.Accounts)
	}
	if len(resp.Folders) != 1 || resp.Folders[0].ID != "f1" {
		t.Fatalf("folders = %+v", resp.Folders)
	}
	if len(resp.Views) != 1 || resp.Views[0].ID != "vf1" {
		t.Fatalf("views = %+v", resp.Views)
	}
	if resp.UnreadCounts["vf1"] != 3 {
		t.Fatalf("unreadCounts = %+v", resp.UnreadCounts)
	}
	if resp.InitialPage == nil || len(resp.InitialPage.Items) != 1 {
		t.Fatalf("initialPage = %+v", resp.InitialPage)
	}
	if resp.Errors != nil {
		t.Fatalf("unexpected errors: %+v", resp.Errors)
	}
	// The list contract strips message bodies; the snapshot must not leak
	// them either.
	if resp.InitialPage.Items[0].Messages != nil {
		t.Fatalf("initial page leaked message bodies: %+v", resp.InitialPage.Items[0].Messages)
	}
}

func TestWorkspaceBootstrapPartialFailuresAreExplicit(t *testing.T) {
	ops := goodBootstrapOps()
	ops.viewsErr = errors.New("views store down")
	ops.conversationErr = errors.New("list failed")
	ops.folderErr = map[string]error{"acc1": errors.New("folder store down")}
	code, resp := bootstrapRequest(t, newBootstrapServer(ops, true), bootstrapBody)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with explicit section errors", code)
	}
	for _, section := range []string{"views", "initialPage", "folders"} {
		if resp.Errors[section] == "" {
			t.Fatalf("missing explicit error for section %s: %+v", section, resp.Errors)
		}
	}
	if len(resp.Accounts) != 1 || resp.UnreadCounts["vf1"] != 3 {
		t.Fatalf("healthy sections must still be returned: %+v", resp)
	}
}

func TestWorkspaceBootstrapAccountsFailureSkipsFoldersExplicitly(t *testing.T) {
	ops := goodBootstrapOps()
	ops.accountsErr = errors.New("account store down")
	code, resp := bootstrapRequest(t, newBootstrapServer(ops, true), bootstrapBody)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp.Errors["accounts"] == "" || resp.Errors["folders"] == "" {
		t.Fatalf("accounts failure must mark folders as skipped: %+v", resp.Errors)
	}
}

func TestWorkspaceBootstrapRejectsWithoutCapability(t *testing.T) {
	code, _ := bootstrapRequest(t, newBootstrapServer(goodBootstrapOps(), false), bootstrapBody)
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", code)
	}
}

func TestWorkspaceBootstrapBoundsInitialPage(t *testing.T) {
	ops := goodBootstrapOps()
	ops.conversations = nil
	code, resp := bootstrapRequest(t, newBootstrapServer(ops, true),
		`{"initialPage":{"scope":{"kind":"folder","accountId":"acc1","folderId":"f1"},"page":{"limit":99999}}}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp.Errors["initialPage"] == "" {
		t.Fatalf("over-limit page must be rejected like the standalone route: %+v", resp)
	}
	if resp.InitialPage != nil {
		t.Fatalf("rejected page must not be returned")
	}
}

func TestWorkspaceBootstrapFoldersScopedToRequestedAccount(t *testing.T) {
	ops := goodBootstrapOps()
	ops.accounts = append(ops.accounts, &account.Account{ID: "acc2", Email: "b@example.com"})
	ops.folders["acc2"] = []*folder.Folder{{ID: "f2", AccountID: "acc2", Name: "Inbox"}}
	code, resp := bootstrapRequest(t, newBootstrapServer(ops, true),
		`{"foldersAccountId":"acc2","initialPage":{"scope":{"kind":"all_inboxes"},"page":{"limit":5}}}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(resp.Folders) != 1 || resp.Folders[0].ID != "f2" {
		t.Fatalf("folders = %+v, want only acc2's", resp.Folders)
	}
}

func TestBootstrapInitialPageIsOptional(t *testing.T) {
	for _, body := range []string{`{}`, `{"initialPage":null}`} {
		status, resp := bootstrapRequest(t, newBootstrapServer(goodBootstrapOps(), true), body)
		if status != http.StatusOK || resp.InitialPage != nil || len(resp.Errors) != 0 {
			t.Fatalf("catalog-only body %s: status=%d response=%+v", body, status, resp)
		}
	}
	_, resp := bootstrapRequest(t, newBootstrapServer(goodBootstrapOps(), true), `{"initialPage":{}}`)
	if resp.Errors["initialPage"] == "" {
		t.Fatal("explicit invalid page must report its section error")
	}
}

type keysetBootstrapProbe struct {
	*stubBootstrapOps
	calls int
}

func (p *keysetBootstrapProbe) ListConversationsUnifiedInboxKeyset(string, int) ([]*message.Conversation, string, error) {
	p.calls++
	return []*message.Conversation{{ThreadID: "keyset"}}, "", nil
}
func (p *keysetBootstrapProbe) ListConversationsByFolderKeyset(string, string, int) ([]*message.Conversation, string, error) {
	panic("unexpected folder")
}
func (p *keysetBootstrapProbe) ListConversationsUnifiedFolderKeyset(string, string, int) ([]*message.Conversation, string, error) {
	panic("unexpected unified folder")
}
func TestBootstrapInitialPageUsesKeyset(t *testing.T) {
	ops := &keysetBootstrapProbe{stubBootstrapOps: goodBootstrapOps()}
	status, resp := bootstrapRequest(t, newBootstrapServer(ops, true), `{"initialPage":{"scope":{"kind":"all_inboxes"},"page":{"limit":50}}}`)
	if status != 200 || ops.calls != 1 || resp.InitialPage == nil || len(resp.InitialPage.Items) != 1 || resp.InitialPage.Items[0].ThreadID != "keyset" {
		t.Fatalf("status %d calls %d response %+v", status, ops.calls, resp)
	}
}
