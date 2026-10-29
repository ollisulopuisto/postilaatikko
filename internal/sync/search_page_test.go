package sync

import (
	"context"
	"fmt"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	appimap "github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/message"
	"net"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSearchUIDPagesReachOlderMatches(t *testing.T) {
	uids := []uint32{3, 9, 1, 7, 5}
	first, next := searchUIDPage(uids, 0, 2)
	if !reflect.DeepEqual(first, []uint32{9, 7}) || next != 7 {
		t.Fatalf("first %v %d", first, next)
	}
	second, next := searchUIDPage(uids, next, 2)
	if !reflect.DeepEqual(second, []uint32{5, 3}) || next != 3 {
		t.Fatalf("second %v %d", second, next)
	}
	last, next := searchUIDPage(uids, next, 2)
	if !reflect.DeepEqual(last, []uint32{1}) || next != 0 {
		t.Fatalf("last %v %d", last, next)
	}
	if !reflect.DeepEqual(uids, []uint32{3, 9, 1, 7, 5}) {
		t.Fatal("mutated caller UIDs")
	}
}

func TestSearchPageRejectsMailboxResetAndWrongScope(t *testing.T) {
	c := IMAPSearchCursor{AccountID: "a", FolderID: "f", Query: "q", UIDValidity: 42, Before: 9}
	if err := validateSearchCursor(c, "a", "f", "q", 42); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		account, folder, query string
		epoch                  uint32
	}{
		{"a", "f", "q", 43}, {"b", "f", "q", 42}, {"a", "g", "q", 42}, {"a", "f", "other", 42},
	} {
		if err := validateSearchCursor(c, test.account, test.folder, test.query, test.epoch); err == nil {
			t.Fatalf("accepted stale scope: %+v", test)
		}
	}
}

// A protocol-level fixture verifies actual UID continuation and that server
// search never asks for body or attachment content.
type headerOnlySession struct {
	imapserver.Session
	t *testing.T
}

func (s *headerOnlySession) Fetch(w *imapserver.FetchWriter, set imap.NumSet, opts *imap.FetchOptions) error {
	if len(opts.BodySection) > 0 || len(opts.BinarySection) > 0 {
		s.t.Error("search fetched message content")
	}
	return s.Session.Fetch(w, set, opts)
}

func TestIMAPSearchPageProtocolHeadersOnlyAndCancellation(t *testing.T) {
	memory := imapmemserver.New()
	user := imapmemserver.NewUser("user", "pass")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	memory.AddUser(user)
	for i := 1; i <= 5; i++ {
		raw := fmt.Sprintf("From: Sender <a@example.org>\r\nTo: b@example.org\r\nMessage-ID: <message%d@example.org>\r\nSubject: Match %d\r\n\r\nPrivate body", i, i)
		if _, err := user.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	srv := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return &headerOnlySession{Session: memory.NewSession(), t: t}, nil, nil
	}, Caps: imap.CapSet{imap.CapIMAP4rev1: {}}, InsecureAuth: true})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	pool := appimap.NewPool(appimap.PoolConfig{MaxConnections: 1, WaiterTimeout: time.Second}, func(string) (*appimap.ClientConfig, error) {
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
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type) VALUES('a','Mail','a@example.org','example.org','example.org','user','password')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','a','Inbox','INBOX','inbox')`); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(pool, account.NewStore(db), folder.NewStore(db), message.NewStore(db), message.NewAttachmentStore(db))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cursor *IMAPSearchCursor
	var got []uint32
	for page := 0; page < 3; page++ {
		result, err := engine.SearchIMAPPage(ctx, "a", "f", "Match", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range result.Results {
			got = append(got, hit.UID)
		}
		cursor = result.Next
		if page < 2 && cursor == nil {
			t.Fatal("older results were truncated")
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if !reflect.DeepEqual(got, []uint32{1, 2, 3, 4, 5}) || cursor != nil {
		t.Fatalf("pages %v next %+v", got, cursor)
	}
	expired := IMAPSearchCursor{AccountID: "a", FolderID: "f", Query: "Match", UIDValidity: 1, Before: 3}
	// Deliberately use an epoch different from the server's actual one.
	first, err := engine.SearchIMAPPage(ctx, "a", "f", "Match", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	expired.UIDValidity = first.Next.UIDValidity + 1
	if _, err = engine.SearchIMAPPage(ctx, "a", "f", "Match", &expired, 2); err == nil {
		t.Fatal("mailbox reset accepted")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err = engine.SearchIMAPPage(cancelled, "a", "f", "", nil, 2); err == nil {
		t.Fatal("cancelled query ran")
	}
}
