package app

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	appimap "github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/message"
	syncengine "github.com/hkdb/aerion/internal/sync"
)

type searchPageSession struct {
	imapserver.Session
	fail *atomic.Bool
	t    *testing.T
}

func (s *searchPageSession) Search(kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	if s.fail.Swap(false) {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "temporary search failure"}
	}
	return s.Session.Search(kind, criteria, options)
}
func (s *searchPageSession) Fetch(w *imapserver.FetchWriter, set imap.NumSet, opts *imap.FetchOptions) error {
	if len(opts.BodySection) > 0 || len(opts.BinarySection) > 0 {
		s.t.Error("continuation downloaded content")
	}
	return s.Session.Fetch(w, set, opts)
}

func TestSearchMailIMAPContinuationAcrossAccountsAndFolders(t *testing.T) {
	memory := imapmemserver.New()
	user := imapmemserver.NewUser("user", "pass")
	for _, mailbox := range []string{"INBOX", "Archive"} {
		if err := user.Create(mailbox, nil); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			raw := fmt.Sprintf("From: Sender <s@example.org>\r\nTo: r@example.org\r\nMessage-ID: <%s%d@example.org>\r\nSubject: Match %d\r\n\r\nPrivate body", mailbox, i, i)
			if _, err := user.Append(mailbox, strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	memory.AddUser(user)
	var fail atomic.Bool
	srv := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return &searchPageSession{Session: memory.NewSession(), fail: &fail, t: t}, nil, nil
	}, Caps: imap.CapSet{imap.CapIMAP4rev1: {}}, InsecureAuth: true})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	pool := appimap.NewPool(appimap.PoolConfig{MaxConnections: 2, WaiterTimeout: time.Second}, func(string) (*appimap.ClientConfig, error) {
		return &appimap.ClientConfig{Host: host, Port: port, Security: appimap.SecurityNone, Username: "user", Password: "pass", AuthMechanism: "login", ConnectTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second}, nil
	})
	defer pool.CloseAll()
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type,enabled) VALUES(?,?,?,'example.org','example.org','user','password',1)`, id, id, id+"@example.org"); err != nil {
			t.Fatal(err)
		}
		for n, mailbox := range []string{"INBOX", "Archive"} {
			if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES(?,?,?,?, 'folder')`, fmt.Sprintf("%s%d", id, n), id, mailbox, mailbox); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := &App{ctx: ctx, accountStore: account.NewStore(db), folderStore: folder.NewStore(db), messageStore: message.NewStore(db)}
	a.syncEngine = syncengine.NewEngine(pool, a.accountStore, a.folderStore, a.messageStore, message.NewAttachmentStore(db))
	req := MailSearchRequest{Query: "Match", Limit: 1}
	seen := map[string]bool{}
	for page := 0; page < 10; page++ {
		result, err := a.SearchMail(req)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Issues) > 0 {
			t.Fatalf("issues: %+v", result.Issues)
		}
		for _, c := range result.Conversations {
			key := c.AccountID + ":" + c.ThreadID
			if seen[key] {
				t.Fatalf("duplicate conversation %q", key)
			}
			seen[key] = true
		}
		req.Cursor = result.Cursor
		if req.Cursor == "" {
			break
		}
	}
	if req.Cursor != "" || len(seen) != 12 {
		t.Fatalf("truncated pages: %d rows, cursor %q", len(seen), req.Cursor)
	}
	// A failed account page must remain retryable at precisely the same cursor.
	req = MailSearchRequest{Query: "Match", AccountIDs: []string{"a"}, Limit: 1}
	first, err := a.SearchMail(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Cursor = first.Cursor
	fail.Store(true)
	failed, err := a.SearchMail(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed.Issues) != 1 || failed.Cursor != first.Cursor {
		t.Fatalf("failure advanced continuation: %+v", failed)
	}
	retried, err := a.SearchMail(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.Issues) != 0 || len(retried.Conversations) != 1 || retried.Conversations[0].ThreadID == first.Conversations[0].ThreadID {
		t.Fatalf("retry skipped or repeated results: %+v", retried)
	}
}
