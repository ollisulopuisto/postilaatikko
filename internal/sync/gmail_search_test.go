package sync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
)

func TestGmailFolderScope(t *testing.T) {
	tests := []struct {
		name       string
		folder     folder.Folder
		wantLabels []string
		wantClause string
		wantErr    error
	}{
		{"inbox", folder.Folder{Type: folder.TypeInbox, Path: "INBOX"}, []string{"INBOX"}, "", nil},
		{"sent", folder.Folder{Type: folder.TypeSent, Path: "[Gmail]/Sent Mail"}, []string{"SENT"}, "", nil},
		{"drafts", folder.Folder{Type: folder.TypeDrafts, Path: "[Gmail]/Drafts"}, []string{"DRAFT"}, "", nil},
		{"trash", folder.Folder{Type: folder.TypeTrash, Path: "[Gmail]/Trash"}, []string{"TRASH"}, "", nil},
		{"spam", folder.Folder{Type: folder.TypeSpam, Path: "[Gmail]/Spam"}, []string{"SPAM"}, "", nil},
		{"starred", folder.Folder{Type: folder.TypeStarred, Path: "[Gmail]/Starred"}, []string{"STARRED"}, "", nil},
		{"all mail has no scope", folder.Folder{Type: folder.TypeAll, Path: "[Gmail]/All Mail"}, nil, "", nil},
		{"user label", folder.Folder{Type: folder.TypeFolder, Path: "Work"}, nil, "label:work", nil},
		{"label with spaces", folder.Folder{Type: folder.TypeFolder, Path: "Old Clients"}, nil, "label:old-clients", nil},
		{"nested label", folder.Folder{Type: folder.TypeFolder, Path: "Work/Q3 Plans"}, nil, "label:work/q3-plans", nil},
		{"unknown system folder falls back", folder.Folder{Type: folder.TypeFolder, Path: "[Gmail]/Important"}, nil, "", gmail.ErrNotCapable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels, clause, err := gmailFolderScope(&tt.folder)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if strings.Join(labels, ",") != strings.Join(tt.wantLabels, ",") || clause != tt.wantClause {
				t.Errorf("labels=%v clause=%q, want %v %q", labels, clause, tt.wantLabels, tt.wantClause)
			}
		})
	}
}

func TestBuildGmailQuery(t *testing.T) {
	if got := buildGmailQuery("  from:bob has:attachment ", "label:work"); got != "(from:bob has:attachment) label:work" {
		t.Errorf("got %q", got)
	}
	if got := buildGmailQuery("invoice", ""); got != "(invoice)" {
		t.Errorf("got %q", got)
	}
}

func TestMessageIDCriteria(t *testing.T) {
	c := messageIDCriteria([]string{"<a@x>", "b@y"})
	if len(c.Or) != 1 {
		t.Fatalf("expected one OR pair, got %+v", c)
	}
	left := c.Or[0][0].Header[0]
	if left.Key != "Message-ID" || left.Value != "a@x" {
		t.Errorf("left = %+v (angle brackets must be stripped)", left)
	}
	right := c.Or[0][1].Header[0]
	if right.Key != "Message-ID" || right.Value != "b@y" {
		t.Errorf("right = %+v", right)
	}
	single := messageIDCriteria([]string{"<only@x>"})
	if len(single.Or) != 0 || single.Header[0].Value != "only@x" {
		t.Errorf("single = %+v", single)
	}
	var _ *imap.SearchCriteria = single
}

func gmailTestEngine(t *testing.T, h http.HandlerFunc) *Engine {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	e := &Engine{gmailBaseURL: srv.URL, gmailBlocked: map[string]time.Time{}}
	e.SetGmailTokenSource(func(ctx context.Context, accountID string, force bool) (string, error) { return "tok", nil })
	return e
}

func TestGmailRFCIDsScopesQueryAndCollectsMessageIDs(t *testing.T) {
	var gotQ, gotLabels string
	e := gmailTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/messages" {
			gotQ = r.URL.Query().Get("q")
			gotLabels = strings.Join(r.URL.Query()["labelIds"], ",")
			_, _ = w.Write([]byte(`{"messages":[{"id":"g1"},{"id":"g2"}],"resultSizeEstimate":7}`))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/messages/")
		mid := "<" + id + "@example.com>"
		if id == "g2" {
			mid = ""
		}
		_, _ = w.Write([]byte(`{"id":"` + id + `","payload":{"headers":[{"name":"Message-ID","value":"` + mid + `"}]}}`))
	})
	f := &folder.Folder{Type: folder.TypeInbox, Path: "INBOX"}
	ids, total, err := e.gmailRFCIDs(context.Background(), "acc", f, "from:bob", 50)
	if err != nil {
		t.Fatalf("gmailRFCIDs: %v", err)
	}
	if gotQ != "(from:bob)" || gotLabels != "INBOX" {
		t.Errorf("q=%q labels=%q", gotQ, gotLabels)
	}
	if len(ids) != 1 || ids[0] != "<g1@example.com>" {
		t.Errorf("ids = %v (messages without a Message-ID header are skipped)", ids)
	}
	if total != 7 {
		t.Errorf("total = %d, want the API estimate 7", total)
	}
}

func TestGmailSearchWithoutTokenSourceIsNotCapable(t *testing.T) {
	e := &Engine{gmailBlocked: map[string]time.Time{}}
	_, err := e.GmailSearch(context.Background(), "acc", "folder", "x", 10)
	if !errors.Is(err, gmail.ErrNotCapable) {
		t.Fatalf("err = %v", err)
	}
}

func TestGmailBlockOnlyForCapabilityErrors(t *testing.T) {
	e := &Engine{gmailBlocked: map[string]time.Time{}}
	e.noteGmailError("a", gmail.ErrRateLimited)
	if e.gmailIsBlocked("a") {
		t.Error("a rate limit must not block the account")
	}
	e.noteGmailError("a", gmail.ErrAPIDisabled)
	if !e.gmailIsBlocked("a") {
		t.Error("API disabled must block the account for a while")
	}
	e.gmailBlocked["a"] = time.Now().Add(-time.Second)
	if e.gmailIsBlocked("a") {
		t.Error("block must expire")
	}
}
