package app

import (
	"context"
	"encoding/json"
	automationclient "github.com/ollisulopuisto/postilaatikko/internal/automation/client"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/views"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopFirstConversationWindowOptsIntoKeyset(t *testing.T) {
	for _, offset := range []int{0, 50} {
		t.Run(string(rune('A'+offset)), func(t *testing.T) {
			var cursor string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req v1.ConversationListRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				cursor = req.Page.Cursor
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"items":[],"complete":true}}`))
			}))
			defer server.Close()
			a := &App{ctx: context.Background(), mailboxService: &mailboxServiceConnector{client: automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "test"}), statusFunc: func(context.Context) MailboxServiceStatus { return MailboxServiceStatus{Connected: true} }}}
			if _, handled, err := a.listServiceConversations(views.Scope{Kind: views.ScopeAllInboxes}, offset, 50, "newest", ""); err != nil || !handled {
				t.Fatalf("handled %v err %v", handled, err)
			}
			want := ""
			if offset != 0 {
				want = "50"
			}
			if cursor != want {
				t.Fatalf("cursor %q want %q", cursor, want)
			}
		})
	}
}
