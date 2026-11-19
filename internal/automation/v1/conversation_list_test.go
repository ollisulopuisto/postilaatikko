package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
)

// stubConversationListOps implements ConversationListOperations with counters
// so tests can tell the offset and keyset paths apart.
type stubConversationListOps struct {
	offsetCalls int
	keysetCalls int
	keysetErr   error
}

func (s *stubConversationListOps) GetFolder(folderID string) (*folder.Folder, error) {
	return &folder.Folder{ID: folderID, AccountID: "account-a"}, nil
}

func (s *stubConversationListOps) ListConversationsByFolder(folderID string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	s.offsetCalls++
	return []*message.Conversation{}, nil
}

func (s *stubConversationListOps) ListConversationsUnifiedInbox(offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	s.offsetCalls++
	return []*message.Conversation{}, nil
}

func (s *stubConversationListOps) ListConversationsUnifiedFolder(folderType string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	s.offsetCalls++
	return []*message.Conversation{}, nil
}

func (s *stubConversationListOps) ListConversationsByFolderKeyset(folderID, cursor string, limit int) ([]*message.Conversation, string, error) {
	s.keysetCalls++
	return []*message.Conversation{{ThreadID: "t1"}}, message.EncodeConversationCursor("2026-11-01 10:00:00", "account-a", "t1", "newest"), s.keysetErr
}

func (s *stubConversationListOps) ListConversationsUnifiedInboxKeyset(cursor string, limit int) ([]*message.Conversation, string, error) {
	s.keysetCalls++
	return []*message.Conversation{{ThreadID: "t1"}}, message.EncodeConversationCursor("2026-11-01 10:00:00", "account-a", "t1", "newest"), s.keysetErr
}

func (s *stubConversationListOps) ListConversationsUnifiedFolderKeyset(folderType, cursor string, limit int) ([]*message.Conversation, string, error) {
	s.keysetCalls++
	return []*message.Conversation{{ThreadID: "t1"}}, message.EncodeConversationCursor("2026-11-01 10:00:00", "account-a", "t1", "newest"), s.keysetErr
}

func postConversationList(t *testing.T, ops ConversationListOperations, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	MountConversationListRoute(mux, ops, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	request := httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(body))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestConversationListUsesKeysetForUnfilteredNewest(t *testing.T) {
	ops := &stubConversationListOps{}
	response := postConversationList(t, ops, `{"scope":{"kind":"all_inboxes"},"page":{"limit":25}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if ops.keysetCalls != 1 || ops.offsetCalls != 0 {
		t.Fatalf("keyset=%d offset=%d, want the keyset path", ops.keysetCalls, ops.offsetCalls)
	}
	var result Response[ConversationListPage]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Complete || !strings.HasPrefix(result.Data.NextCursor, message.KeysetCursorPrefix) {
		t.Fatalf("page = %+v, want an incomplete page with a keyset cursor", result.Data)
	}
}

func TestConversationListKeepsOffsetForLegacyCursorsAndFilters(t *testing.T) {
	for name, body := range map[string]string{
		"numeric cursor": `{"scope":{"kind":"all_inboxes"},"page":{"limit":25,"cursor":"50"}}`,
		"filtered":       `{"scope":{"kind":"all_inboxes"},"filter":"unread","page":{"limit":25}}`,
		"oldest":         `{"scope":{"kind":"all_inboxes"},"sortOrder":"oldest","page":{"limit":25}}`,
		"archive":        `{"scope":{"kind":"unified_folder","folderType":"archive"},"page":{"limit":25}}`,
		"read later":     `{"scope":{"kind":"unified_folder","folderType":"readLater"},"page":{"limit":25}}`,
	} {
		t.Run(name, func(t *testing.T) {
			ops := &stubConversationListOps{}
			response := postConversationList(t, ops, body)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if ops.keysetCalls != 0 || ops.offsetCalls != 1 {
				t.Fatalf("keyset=%d offset=%d, want the offset path", ops.keysetCalls, ops.offsetCalls)
			}
		})
	}
}

func TestConversationListKeysetEligibleUnifiedFolders(t *testing.T) {
	for _, folderType := range []string{"drafts", "sent", "trash", "starred", "spam"} {
		ops := &stubConversationListOps{}
		response := postConversationList(t, ops, `{"scope":{"kind":"unified_folder","folderType":"`+folderType+`"},"page":{"limit":25}}`)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", folderType, response.Code, response.Body.String())
		}
		if ops.keysetCalls != 1 {
			t.Fatalf("%s: keyset=%d, want the keyset path", folderType, ops.keysetCalls)
		}
	}
}

func TestConversationListRejectsMalformedKeysetCursor(t *testing.T) {
	ops := &stubConversationListOps{keysetErr: errors.New("malformed keyset cursor")}
	response := postConversationList(t, ops, `{"scope":{"kind":"all_inboxes"},"page":{"limit":25,"cursor":"k1:not-base64!!"}}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want bad request", response.Code, response.Body.String())
	}
}

func TestConversationListFolderScopeKeysetStampsIdentity(t *testing.T) {
	ops := &stubConversationListOps{}
	response := postConversationList(t, ops, `{"scope":{"kind":"folder","accountId":"account-a","folderId":"folder-a"},"page":{"limit":25}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if ops.keysetCalls != 1 {
		t.Fatalf("keyset=%d, want the keyset path", ops.keysetCalls)
	}
	var result Response[ConversationListPage]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Items) != 1 || result.Data.Items[0].AccountID != "account-a" || result.Data.Items[0].FolderID != "folder-a" {
		t.Fatalf("items = %+v, want account/folder stamped", result.Data.Items)
	}
}
