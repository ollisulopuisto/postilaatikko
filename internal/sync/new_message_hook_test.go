package sync

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	appimap "github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/message"
)

// hookTestIMAPServer serves the minimal IMAP exchange for fetchMessageHeaders:
// greeting, CAPABILITY, LOGIN, SELECT, UID FETCH (ENVELOPE ...), LOGOUT.
type hookTestIMAPServer struct {
	listener net.Listener
}

// cannedRaw is the RFC822 header content returned for the test message.
const cannedRaw = "From: bob@example.com\r\nTo: me@corp.example\r\nSubject: invoice #42\r\nMessage-ID: <m1@example.com>\r\n\r\nbody line\r\n"

func newHookTestIMAPServer(t *testing.T) *hookTestIMAPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &hookTestIMAPServer{listener: listener}
	t.Cleanup(func() { _ = listener.Close() })
	go server.serve()
	return server
}

func (s *hookTestIMAPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *hookTestIMAPServer) handle(conn net.Conn) {
	defer conn.Close()
	_, _ = fmt.Fprintf(conn, "* OK [CAPABILITY IMAP4rev1] hook test server\r\n")
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		tag := fields[0]
		upper := strings.ToUpper(line)
		switch {
		case strings.Contains(upper, " CAPABILITY"):
			_, _ = fmt.Fprintf(conn, "* CAPABILITY IMAP4rev1\r\n%s OK CAPABILITY completed\r\n", tag)
		case strings.Contains(upper, " LOGIN "):
			_, _ = fmt.Fprintf(conn, "%s OK LOGIN completed\r\n", tag)
		case strings.Contains(upper, " SELECT "):
			_, _ = fmt.Fprintf(conn, "* FLAGS (\\Seen)\r\n* 0 EXISTS\r\n* OK [UIDVALIDITY 1] valid\r\n* OK [UIDNEXT 1000] next\r\n%s OK [READ-WRITE] SELECT completed\r\n", tag)
		case strings.Contains(upper, " UID FETCH "):
			uid := fields[3]
			// ENVELOPE order as parsed by go-imap: date subject from sender
			// reply-to to cc bcc in-reply-to message-id; each address is
			// (name route mailbox host).
			// Return the header section too (real servers always do): the app
			// classifies the inbox category from these headers and stores it
			// NOT NULL.
			headerLiteral := fmt.Sprintf("BODY[HEADER] {%d}\r\n%s", len(cannedRaw), cannedRaw)
			_, _ = fmt.Fprintf(conn,
				"* 1 FETCH (UID %s FLAGS () RFC822.SIZE %d INTERNALDATE \"17-Oct-2026 12:00:00 +0000\" ENVELOPE (\"17-Oct-2026 12:00:00 +0000\" \"Test subject\" ((\"Bob\" NIL \"bob\" \"example.com\")) NIL NIL ((\"Me\" NIL \"me\" \"example.com\")) NIL NIL NIL \"<m1@example.com>\") %s)\r\n%s OK FETCH completed\r\n",
				uid, len(cannedRaw), headerLiteral, tag)
		case strings.Contains(upper, " LOGOUT"):
			_, _ = fmt.Fprintf(conn, "* BYE logging out\r\n%s OK LOGOUT completed\r\n", tag)
			return
		default:
			_, _ = fmt.Fprintf(conn, "%s OK completed\r\n", tag)
		}
	}
}

func (s *hookTestIMAPServer) credentials(accountID string) (*appimap.ClientConfig, error) {
	host, portText, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	return &appimap.ClientConfig{
		Host:           host,
		Port:           port,
		Security:       appimap.SecurityNone,
		Username:       accountID,
		Password:       "test",
		AuthMechanism:  "login",
		ConnectTimeout: time.Second,
		ReadTimeout:    time.Second,
		WriteTimeout:   time.Second,
	}, nil
}

func TestFetchMessageHeadersFiresNewMessageHandlerOncePerNewUID(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "hook.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"INSERT INTO accounts (id,name,email,imap_host,smtp_host,username,sync_period_days) VALUES ('account','Test','test@example.com','127.0.0.1','smtp','test',0)",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('inbox','account','Inbox','INBOX','inbox')",
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}

	server := newHookTestIMAPServer(t)
	pool := appimap.NewPool(appimap.PoolConfig{
		MaxConnections: 1,
		IdleTimeout:    time.Minute,
		ConnectTimeout: time.Second,
		WaiterTimeout:  time.Second,
	}, server.credentials)
	t.Cleanup(pool.CloseAll)

	messageStore := message.NewStore(db)
	engine := NewEngine(pool, account.NewStore(db), folder.NewStore(db), messageStore, message.NewAttachmentStore(db))

	var fired []uint32
	engine.SetNewMessageHandler(func(accountID, folderID string, m *message.Message) {
		fired = append(fired, m.UID)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pool.GetConnection(ctx, "account")
	if err != nil {
		t.Fatal(err)
	}
	client := conn.Client().RawClient()
	defer pool.Release(conn)

	var persist time.Duration

	n, err := engine.fetchMessageHeaders(ctx, client, "account", "inbox", []uint32{10}, &persist)
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Expected 1 persisted message, got %d", n)
	}
	if len(fired) != 1 || fired[0] != 10 {
		t.Fatalf("Expected handler fired once for UID 10, got %v", fired)
	}

	// Re-fetching the same UID is not a new message and must not fire again.
	n, err = engine.fetchMessageHeaders(ctx, client, "account", "inbox", []uint32{10}, &persist)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Expected 1 upserted message, got %d", n)
	}
	if len(fired) != 1 {
		t.Fatalf("Handler fired for an existing UID: %v", fired)
	}

	// A genuinely new UID fires the handler again.
	n, err = engine.fetchMessageHeaders(ctx, client, "account", "inbox", []uint32{11}, &persist)
	if err != nil {
		t.Fatalf("Third fetch failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Expected 1 persisted message, got %d", n)
	}
	if len(fired) != 2 || fired[1] != 11 {
		t.Fatalf("Expected handler fired for UID 11, got %v", fired)
	}
}
