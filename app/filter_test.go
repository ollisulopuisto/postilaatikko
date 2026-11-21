package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/mail"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	appimap "github.com/ollisulopuisto/postilaatikko/internal/imap"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	syncre "github.com/ollisulopuisto/postilaatikko/internal/sync"
	"github.com/ollisulopuisto/postilaatikko/internal/undo"
)

// cannedRaw is the RFC822 payload served by the fake source server for the
// message under test and expected to be APPENDed to the destination account.
const cannedRaw = "From: bob@example.com\r\nTo: me@corp.example\r\nSubject: invoice #42\r\nMessage-ID: <m1@example.com>\r\n\r\nbody line\r\n"

// filterOpsRecorder is a shared, ordered operation log so tests can assert the
// cross-server ordering of IMAP mutations (append before source deletion).
type filterOpsRecorder struct {
	mu  stdsync.Mutex
	ops []string
}

func (r *filterOpsRecorder) record(op string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, op)
}

func (r *filterOpsRecorder) indexOf(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, op := range r.ops {
		if strings.HasPrefix(op, prefix) {
			return i
		}
	}
	return -1
}

// testIMAPSession implements imapserver.Session for testing. It stores
// messages in memory keyed by UID and records IMAP operations in the shared
// ops log.
type testIMAPSession struct {
	ops              *filterOpsRecorder
	mu               stdsync.Mutex
	messages         map[uint32][]byte // UID -> raw RFC822
	nextUID          uint32
	selected         string
	storeDelay       time.Duration
	copyBlock        map[uint32]<-chan struct{}
	copyStarted      chan uint32
	forceFallback    bool
	rediscoverCopies bool
	internalDate     time.Time
	searchError      bool
	fetchError       bool
	copiedUIDs       map[uint32]string
	deleteSource     bool
	afterExpunge     func() bool
	copyCount        int
	validity         uint32
	mailboxValidity  map[string]uint32
	resetOnCopy      uint32
	copyValidity     uint32
	rejectAppend     bool // when true, Append is rejected after recording the attempt
}

func newTestIMAPSession(ops *filterOpsRecorder) *testIMAPSession {
	return &testIMAPSession{
		ops:      ops,
		messages: make(map[uint32][]byte),
		nextUID:  100,
	}
}

func (s *testIMAPSession) Close() error { return nil }

func (s *testIMAPSession) Login(username, password string) error { return nil }

func (s *testIMAPSession) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = mailbox
	return &imap.SelectData{
		UIDValidity: 1,
		UIDNext:     imap.UID(s.nextUID),
		NumMessages: uint32(len(s.messages)),
	}, nil
}

func (s *testIMAPSession) Status(mailbox string, options *imap.StatusOptions) (*imap.StatusData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	zero := uint32(0)
	count := zero
	if s.rediscoverCopies {
		for _, owner := range s.copiedUIDs {
			if owner == mailbox {
				count++
			}
		}
	}
	return &imap.StatusData{Mailbox: mailbox, NumMessages: &count, NumUnseen: &zero, NumDeleted: &zero, UIDValidity: 1, UIDNext: imap.UID(s.nextUID)}, nil
}

func (s *testIMAPSession) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.nextUID++
	s.messages[s.nextUID] = data
	s.mu.Unlock()
	s.ops.record("append:" + mailbox)
	if s.rejectAppend {
		return nil, fmt.Errorf("append rejected for test")
	}
	return &imap.AppendData{UID: imap.UID(s.nextUID)}, nil
}

// Fetch responds to FETCH commands with the stored messages. Each requested
// UID is returned with its full RFC822 content as a body section.
func (s *testIMAPSession) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	return s.fetchMailbox("", w, numSet, options)
}
func (s *testIMAPSession) fetchMailbox(mailbox string, w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	s.mu.Lock()
	failed := s.fetchError
	s.mu.Unlock()
	if failed {
		return fmt.Errorf("synthetic fetch failure")
	}
	s.mu.Lock()
	messages := make(map[uint32][]byte, len(s.messages))
	for uid, raw := range s.messages {
		if s.rediscoverCopies && mailbox != "" {
			owner := s.copiedUIDs[uid]
			if owner == "" {
				owner = "INBOX"
			}
			if owner != mailbox {
				continue
			}
		}
		messages[uid] = raw
	}
	s.mu.Unlock()

	fixedDate := s.internalDate
	if fixedDate.IsZero() {
		fixedDate = time.Now().UTC()
	}

	// Collect requested UIDs from the num set (UID FETCH or SEQ FETCH).
	var wanted []uint32
	if uidSet, ok := numSet.(imap.UIDSet); ok {
		for _, rng := range uidSet {
			for uid := rng.Start; uid <= rng.Stop; uid++ {
				wanted = append(wanted, uint32(uid))
			}
		}
	} else if seqSet, ok := numSet.(imap.SeqSet); ok {
		for _, rng := range seqSet {
			for seq := rng.Start; seq <= rng.Stop; seq++ {
				wanted = append(wanted, seq)
			}
		}
	}

	for _, uid := range wanted {
		raw, ok := messages[uid]
		if !ok {
			continue
		}
		mw := w.CreateMessage(uid)
		mw.WriteUID(imap.UID(uid))
		mw.WriteRFC822Size(int64(len(raw)))
		mw.WriteInternalDate(fixedDate)
		mw.WriteFlags(nil)
		if options.Envelope {
			msg, err := mail.ReadMessage(bytes.NewReader(raw))
			if err != nil {
				return err
			}
			mw.WriteEnvelope(&imap.Envelope{Date: fixedDate, Subject: msg.Header.Get("Subject"), MessageID: strings.Trim(msg.Header.Get("Message-ID"), "<>"), From: []imap.Address{{Mailbox: "bulk", Host: "example.test"}}, To: []imap.Address{{Mailbox: "me", Host: "example.test"}}})
		}
		for _, bs := range options.BodySection {
			wc := mw.WriteBodySection(bs, int64(len(raw)))
			if _, err := wc.Write(raw); err != nil {
				wc.Close()
				return err
			}
			if err := wc.Close(); err != nil {
				return err
			}
		}
		if err := mw.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (s *testIMAPSession) Search(kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	return s.searchMailbox(s.selected, kind, criteria, options)
}
func (s *testIMAPSession) searchMailbox(mailbox string, kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	s.mu.Lock()
	failed := s.searchError
	s.mu.Unlock()
	if failed {
		return nil, fmt.Errorf("synthetic search failure")
	}
	if kind == imapserver.NumKindUID {
		result := imap.UIDSet{}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.rediscoverCopies {
			var uids []int
			for uid, owner := range s.copiedUIDs {
				date := s.internalDate
				if date.IsZero() {
					date = time.Now().UTC()
				}
				if !criteria.Since.IsZero() && criteria.Since.After(date) {
					continue
				}
				if len(criteria.UID) > 0 && !criteria.UID[0].Contains(imap.UID(uid)) {
					continue
				}
				if owner != mailbox {
					continue
				}
				match := true
				for _, header := range criteria.Header {
					m, err := mail.ReadMessage(bytes.NewReader(s.messages[uid]))
					if err != nil {
						return nil, err
					}
					if !strings.EqualFold(m.Header.Get(header.Key), header.Value) {
						match = false
					}
				}
				if match {
					uids = append(uids, int(uid))
				}
			}
			sort.Ints(uids)
			for _, uid := range uids {
				result.AddNum(imap.UID(uid))
			}
		}
		return &imap.SearchData{All: result}, nil
	}
	return &imap.SearchData{All: imap.SeqSet{}}, nil
}

func (s *testIMAPSession) Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error) {
	if set, ok := numSet.(imap.UIDSet); ok && len(set) > 0 {
		uid := uint32(set[0].Start)
		if s.copyStarted != nil {
			s.copyStarted <- uid
		}
		if gate := s.copyBlock[uid]; gate != nil {
			<-gate
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.copyCount++
	var wanted []uint32
	if uidSet, ok := numSet.(imap.UIDSet); ok {
		for _, rng := range uidSet {
			for uid := rng.Start; uid <= rng.Stop; uid++ {
				wanted = append(wanted, uint32(uid))
			}
		}
	} else if seqSet, ok := numSet.(imap.SeqSet); ok {
		for _, rng := range seqSet {
			for seq := rng.Start; seq <= rng.Stop; seq++ {
				wanted = append(wanted, seq)
			}
		}
	}
	var sourceUIDs, destUIDs imap.UIDSet
	for _, uid := range wanted {
		if raw, ok := s.messages[uid]; ok {
			s.nextUID++
			s.messages[s.nextUID] = raw
			if s.rediscoverCopies {
				if s.copiedUIDs == nil {
					s.copiedUIDs = map[uint32]string{}
				}
				s.copiedUIDs[s.nextUID] = dest
			}
			sourceUIDs = append(sourceUIDs, imap.UIDRange{Start: imap.UID(uid), Stop: imap.UID(uid)})
			destUIDs = append(destUIDs, imap.UIDRange{Start: imap.UID(s.nextUID), Stop: imap.UID(s.nextUID)})
		}
	}
	s.ops.record("copy:" + dest)
	if s.forceFallback {
		return nil, nil
	}
	if s.resetOnCopy != 0 {
		if s.mailboxValidity == nil {
			s.mailboxValidity = map[string]uint32{}
		}
		s.mailboxValidity[dest] = s.resetOnCopy
	}
	validity := s.copyValidity
	if validity == 0 {
		validity = s.mailboxValidity[dest]
	}
	if validity == 0 {
		validity = s.validity
	}
	if validity == 0 {
		validity = 1
	}
	return &imap.CopyData{
		UIDValidity: validity,
		SourceUIDs:  sourceUIDs,
		DestUIDs:    destUIDs,
	}, nil
}

func (s *testIMAPSession) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	time.Sleep(s.storeDelay)
	s.ops.record("store:complete")
	return nil
}

func (s *testIMAPSession) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	return nil
}

func (s *testIMAPSession) Unselect() error                                          { return nil }
func (s *testIMAPSession) Create(mailbox string, options *imap.CreateOptions) error { return nil }
func (s *testIMAPSession) Delete(mailbox string) error                              { return nil }
func (s *testIMAPSession) Rename(mailbox, newName string, options *imap.RenameOptions) error {
	return nil
}
func (s *testIMAPSession) Subscribe(mailbox string) error   { return nil }
func (s *testIMAPSession) Unsubscribe(mailbox string) error { return nil }
func (s *testIMAPSession) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	return nil
}
func (s *testIMAPSession) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error    { return nil }
func (s *testIMAPSession) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error { return nil }

// Every connection has independent selected-mailbox state; messages and COPY
// destinations remain shared provider state under the session mutex.
type testIMAPConnection struct {
	*testIMAPSession
	mailbox string
	conn    net.Conn
	deleted []uint32
}

func (c *testIMAPConnection) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	c.mailbox = mailbox
	c.mu.Lock()
	defer c.mu.Unlock()
	count := uint32(len(c.messages))
	if c.rediscoverCopies {
		count = 0
		for _, owner := range c.copiedUIDs {
			if owner == mailbox {
				count++
			}
		}
	}
	validity := c.mailboxValidity[mailbox]
	if validity == 0 {
		validity = c.validity
	}
	if validity == 0 {
		validity = 1
	}
	return &imap.SelectData{UIDValidity: validity, UIDNext: imap.UID(c.nextUID), NumMessages: count}, nil
}
func (c *testIMAPConnection) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	return c.fetchMailbox(c.mailbox, w, numSet, options)
}
func (c *testIMAPConnection) Search(kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	return c.searchMailbox(c.mailbox, kind, criteria, options)
}

// testIMAPServer wraps an imapserver.Server listening on a loopback port.
type testIMAPServer struct {
	server *imapserver.Server
	ln     net.Listener
	sess   *testIMAPSession
}

func newTestIMAPServer(t *testing.T, ops *filterOpsRecorder) *testIMAPServer {
	t.Helper()
	sess := newTestIMAPSession(ops)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &testIMAPConnection{testIMAPSession: sess, conn: conn.NetConn()}, nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })
	return &testIMAPServer{server: srv, ln: ln, sess: sess}
}

func (s *testIMAPServer) clientConfig(accountID string) (*appimap.ClientConfig, error) {
	host, portText, err := net.SplitHostPort(s.ln.Addr().String())
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
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
	}, nil
}

type filterFixture struct {
	app     *App
	db      *database.DB
	ops     *filterOpsRecorder
	serverA *testIMAPServer
	serverB *testIMAPServer
	message *message.Message
	rule    *mailfilter.Rule
}

// newFilterFixture builds a two-account App: account A (the source, with a
// real sync engine so the move path can fetch raw bytes) and account B (the
// destination). a.ctx is cancelled after 500ms so any delayed SyncFolder
// triggered by the move (e.g. the 750ms move-destination reconcile) bails out
// before it can touch Wails runtime event emission, which requires a live
// Wails context. The destination folder's sync key is pre-seeded in
// syncLastRequest so the immediate post-move destination sync is debounced.
func newFilterFixture(t *testing.T, rejectAppend bool) *filterFixture {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "filters.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"INSERT INTO accounts (id,name,email,imap_host,smtp_host,username,sync_period_days) VALUES ('acct-a','Source','source@example.com','127.0.0.1','smtp','source',30)",
		"INSERT INTO accounts (id,name,email,imap_host,smtp_host,username,sync_period_days) VALUES ('acct-b','Destination','dest@example.com','127.0.0.1','smtp','dest',30)",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('a-inbox','acct-a','Inbox','INBOX','inbox')",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('a-trash','acct-a','Trash','Trash','trash')",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('b-dest','acct-b','Dest','Dest','folder')",
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}

	ops := &filterOpsRecorder{}
	serverA := newTestIMAPServer(t, ops)
	serverB := newTestIMAPServer(t, ops)

	// Pool credentials dispatch by account ID to the matching fake server.
	poolCredentials := func(accountID string) (*appimap.ClientConfig, error) {
		switch accountID {
		case "acct-a":
			return serverA.clientConfig(accountID)
		default:
			return serverB.clientConfig(accountID)
		}
	}
	pool := appimap.NewPool(appimap.PoolConfig{
		MaxConnections: 2,
		IdleTimeout:    time.Minute,
		ConnectTimeout: 5 * time.Second,
		WaiterTimeout:  5 * time.Second,
	}, poolCredentials)
	t.Cleanup(pool.CloseAll)

	accountStore := account.NewStore(db)
	folderStore := folder.NewStore(db)
	messageStore := message.NewStore(db)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	t.Cleanup(cancel)

	app := &App{
		ctx:          ctx,
		db:           db,
		accountStore: accountStore,
		folderStore:  folderStore,
		messageStore: messageStore,
		imapPool:     pool,
		syncEngine:   syncre.NewEngine(pool, accountStore, folderStore, messageStore, message.NewAttachmentStore(db)),
		undoStack:    undo.NewStack(50, 30*time.Second),
		ownExpungeAt: make(map[string]time.Time),
		syncContexts: make(map[string]context.CancelFunc),
		syncLastRequest: map[string]time.Time{
			"acct-b:b-dest": time.Now(),
		},
		moveSyncTimers:      make(map[string]*time.Timer),
		moveSyncWaiters:     make(map[string][]*undo.MoveCompletion),
		runtimeEventEmitter: func(string, ...interface{}) {},
		filterStore:         mailfilter.NewStore(db),
	}

	// Moves return before provider reconciliation and count updates finish.
	// Join their tracked work before closing SQLite/removing its WAL directory.
	t.Cleanup(func() {
		cancel()
		deadline := time.Now().Add(5 * time.Second)
		for {
			app.moveMutationMu.Lock()
			flights := len(app.moveMutationFlights)
			app.moveMutationMu.Unlock()
			app.moveSyncMu.Lock()
			delayed := len(app.moveSyncTimers) + len(app.moveSyncWaiters)
			app.moveSyncMu.Unlock()
			app.syncMu.Lock()
			syncing := len(app.syncContexts)
			app.syncMu.Unlock()
			if flights == 0 && delayed == 0 && syncing == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("filter fixture did not settle: moves=%d delayed=%d syncs=%d", flights, delayed, syncing)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	app.filterRouter = mailfilter.NewAutomaticRouter(app.filterStore, folderStore, func(rule *mailfilter.Rule, destination *folder.Folder, msg *message.Message) {
		go app.applyMatchedFilterRule(rule, msg)
	})

	// Pre-populate server A with the test message (UID 10 in the inbox).
	if err := messageStore.Create(&message.Message{
		ID:        "msg-1",
		AccountID: "acct-a",
		FolderID:  "a-inbox",
		UID:       10,
		MessageID: "<m1@example.com>",
		FromName:  "Bob",
		FromEmail: "bob@example.com",
		ToList:    `[{"name":"Me","email":"me@corp.example"}]`,
		Subject:   "invoice #42",
		Date:      time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	serverA.sess.mu.Lock()
	serverA.sess.messages[10] = []byte(cannedRaw)
	serverA.sess.mu.Unlock()

	rule := &mailfilter.Rule{
		Name:            "Route Bob's invoices",
		Enabled:         true,
		AccountID:       "acct-a",
		FromContains:    []string{"bob@example.com"},
		SubjectContains: []string{"invoice"},
		DestFolderID:    "b-dest",
	}
	if err := app.CreateFilterRule(rule); err != nil {
		t.Fatalf("CreateFilterRule failed: %v", err)
	}

	msg, err := messageStore.GetByIDs([]string{"msg-1"})
	if err != nil || len(msg) == 0 {
		t.Fatal("fixture message missing")
	}

	return &filterFixture{
		app:     app,
		db:      db,
		ops:     ops,
		serverA: serverA,
		serverB: serverB,
		message: msg[0],
		rule:    rule,
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !condition() {
		t.Fatal("condition not met within timeout")
	}
}

func (f *filterFixture) messageFolderID(t *testing.T) string {
	t.Helper()
	messages, err := f.app.messageStore.GetByIDs([]string{"msg-1"})
	if err != nil || len(messages) == 0 {
		t.Fatalf("message no longer resolvable: %v", err)
	}
	return messages[0].FolderID
}

// prependAppendReject makes server B reject every APPEND. Used to test the
// safety property that a failed destination copy leaves the source untouched.
func (f *filterFixture) prependAppendReject(t *testing.T) {
	t.Helper()
	f.serverB.sess.mu.Lock()
	f.serverB.sess.rejectAppend = true
	f.serverB.sess.mu.Unlock()
}

func TestFilterRoutesIncomingMessageAcrossAccounts(t *testing.T) {
	f := newFilterFixture(t, false)

	f.app.handleNewMessageForFilters(f.message)

	// The cross-account move APPENDs to the destination first, then routes the
	// source through the Trash pipeline, so the local row ends up in account A's
	// trash folder. The local move is optimistic, so observing that folder
	// alone does not mean the background source-side COPY has run. Wait for
	// both before checking the remote ordering or tearing down the fixture.
	waitFor(t, 10*time.Second, func() bool {
		return f.messageFolderID(t) == "a-trash" && f.ops.indexOf("copy:") >= 0
	})

	appendIdx := f.ops.indexOf("append:")
	copyIdx := f.ops.indexOf("copy:")
	if appendIdx < 0 {
		t.Fatal("destination APPEND was never issued")
	}
	if copyIdx < 0 {
		t.Fatal("source-side copy was never issued")
	}
	if appendIdx > copyIdx {
		t.Fatalf("destination append (op %d) happened after source deletion (op %d); copy must succeed before source deletion", appendIdx, copyIdx)
	}
}

func TestFilterKeepsSourceWhenAppendFails(t *testing.T) {
	f := newFilterFixture(t, true)
	f.prependAppendReject(t)

	f.app.handleNewMessageForFilters(f.message)

	// Wait until the destination server has seen (and rejected) the append.
	waitFor(t, 10*time.Second, func() bool {
		return f.ops.indexOf("append:") >= 0
	})
	// Give a buggy implementation a short window to touch the source.
	time.Sleep(300 * time.Millisecond)

	if got := f.messageFolderID(t); got != "a-inbox" {
		t.Fatalf("source message moved to %q although the destination append failed", got)
	}
	if f.ops.indexOf("copy:") >= 0 {
		t.Fatal("source-side copy ran although the destination append failed")
	}
}

func TestFilterHandlerIgnoresNonMatchingMessages(t *testing.T) {
	f := newFilterFixture(t, false)

	other := &message.Message{
		ID:        "msg-2",
		AccountID: "acct-a",
		FolderID:  "a-inbox",
		UID:       11,
		MessageID: "<m2@example.com>",
		FromName:  "Carol",
		FromEmail: "carol@example.com",
		Subject:   "hello",
		Date:      time.Now().UTC(),
	}
	if err := f.app.messageStore.Create(other); err != nil {
		t.Fatal(err)
	}
	// Make the unmatched message available on the source server too.
	f.serverA.sess.mu.Lock()
	f.serverA.sess.messages[11] = []byte("From: carol@example.com\r\nSubject: hello\r\n\r\nbody\r\n")
	f.serverA.sess.mu.Unlock()

	f.app.handleNewMessageForFilters(other)
	time.Sleep(300 * time.Millisecond)

	if got := f.messageFolderID(t); got != "a-inbox" {
		t.Fatalf("unmatched message affected source folder: %q", got)
	}
	if f.ops.indexOf("append:") >= 0 {
		t.Fatal("non-matching message triggered a cross-account append")
	}
}

func TestCreateFilterRuleValidation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "filter-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"INSERT INTO accounts (id,name,email,imap_host,smtp_host,username,sync_period_days) VALUES ('acct-a','Source','source@example.com','127.0.0.1','smtp','source',30)",
		"INSERT INTO accounts (id,name,email,imap_host,smtp_host,username,sync_period_days) VALUES ('acct-b','Destination','dest@example.com','127.0.0.1','smtp','dest',30)",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('a-inbox','acct-a','Inbox','INBOX','inbox')",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('a-work','acct-a','Work','Work','folder')",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('b-dest','acct-b','Dest','Dest','folder')",
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{
		ctx:             context.Background(),
		db:              db,
		accountStore:    account.NewStore(db),
		folderStore:     folder.NewStore(db),
		filterStore:     mailfilter.NewStore(db),
		messageStore:    message.NewStore(db),
		syncContexts:    make(map[string]context.CancelFunc),
		syncLastRequest: make(map[string]time.Time),
	}

	t.Run("backend rejects catch-all trash create and update", func(t *testing.T) {
		unsafe := &mailfilter.Rule{Name: "Unsafe", Enabled: true, ActionTrash: true, SubjectContains: []string{" ", "\t"}}
		if err := app.CreateFilterRule(unsafe); err == nil {
			t.Fatal("Wails create accepted catch-all Trash")
		}
		safe := &mailfilter.Rule{Name: "Safe", Enabled: true, ActionTrash: true, SubjectContains: []string{"invoice"}}
		if err := app.CreateFilterRule(safe); err != nil {
			t.Fatal(err)
		}
		safe.SubjectContains = nil
		if err := app.UpdateFilterRule(safe); err == nil {
			t.Fatal("Wails update accepted catch-all Trash")
		}
		stored, err := app.filterStore.Get(safe.ID)
		if err != nil || len(stored.SubjectContains) != 1 {
			t.Fatal("rejected update changed saved criteria")
		}
		if err := app.DeleteFilterRule(safe.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("same-account destination is accepted", func(t *testing.T) {
		rule := &mailfilter.Rule{Name: "same account", FromContains: []string{"sender@example.com"}, AccountID: "acct-a", DestFolderID: "a-work"}
		if err := app.CreateFilterRule(rule); err != nil {
			t.Fatal(err)
		}
		if err := app.DeleteFilterRule(rule.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("missing destination is rejected", func(t *testing.T) {
		rule := &mailfilter.Rule{Name: "missing", AccountID: "acct-a", DestFolderID: "nope"}
		if err := app.CreateFilterRule(rule); err == nil {
			t.Fatal("expected error for missing destination")
		}
	})

	t.Run("missing source account is rejected", func(t *testing.T) {
		rule := &mailfilter.Rule{Name: "ghost account", AccountID: "ghost", DestFolderID: "b-dest"}
		if err := app.CreateFilterRule(rule); err == nil {
			t.Fatal("expected error for unknown source account")
		}
	})

	t.Run("valid cross-account rule is persisted", func(t *testing.T) {
		rule := &mailfilter.Rule{Name: "good", AccountID: "acct-a", FromContains: []string{"x@y"}, DestFolderID: "b-dest"}
		if err := app.CreateFilterRule(rule); err != nil {
			t.Fatalf("CreateFilterRule failed: %v", err)
		}
		if rule.ID == "" {
			t.Fatal("rule ID not assigned")
		}
		rules, err := app.GetFilterRules()
		if err != nil || len(rules) != 1 || rules[0].ID != rule.ID {
			t.Fatalf("GetFilterRules round trip failed: %v %+v", err, rules)
		}
	})
}

func TestFilterWaitsForProviderReadBeforeCopy(t *testing.T) {
	f := newFilterFixture(t, false)
	f.serverA.sess.storeDelay = 100 * time.Millisecond
	f.app.ownFlagChangeAt = make(map[string]time.Time)
	rule, err := f.app.filterStore.Get(f.rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	rule.ActionMarkRead = true
	if err := f.app.UpdateFilterRule(rule); err != nil {
		t.Fatal(err)
	}
	m, err := f.app.messageStore.Get("msg-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.applyFilterActions(rule, []string{m.ID}, m); err != nil {
		t.Fatal(err)
	}
	store := f.ops.indexOf("store:complete")
	copy := f.ops.indexOf("append:")
	if store < 0 || copy < 0 || store > copy {
		t.Fatalf("read not confirmed before copy: store %d append %d", store, copy)
	}
}

func TestReadOnlyFilterRefreshesFolderBadge(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ownFlagChangeAt = make(map[string]time.Time)
	if err := f.app.folderStore.UpdateCounts("a-inbox", 1, 1); err != nil {
		t.Fatal(err)
	}
	countEvents := 0
	f.app.runtimeEventEmitter = func(name string, args ...interface{}) {
		if name == "folders:countsChanged" {
			countEvents++
		}
	}
	rule := *f.rule
	rule.DestFolderID = ""
	rule.ActionMarkRead = true
	if n, err := f.app.applyFilterActions(&rule, []string{f.message.ID}, f.message); err != nil || n != 1 {
		t.Fatalf("apply %d %v", n, err)
	}
	folder, err := f.app.folderStore.Get("a-inbox")
	if err != nil || folder.UnreadCount != 0 || countEvents != 1 {
		t.Fatalf("badge not refreshed: %+v events %d error %v", folder, countEvents, err)
	}
}

func TestBulkFilterFallbackPreservesOtherMoveLocalState(t *testing.T) {
	for _, count := range []int{2, 9} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := newFilterFixture(t, false)
			f.app.ctx = context.Background()
			f.serverA.sess.forceFallback = true
			f.serverA.sess.rediscoverCopies = true
			f.serverA.sess.nextUID = 1000
			started := make(chan uint32, 16)
			f.serverA.sess.copyStarted = started
			gate := make(chan struct{})
			var once stdsync.Once
			release := func() { once.Do(func() { close(gate) }) }
			defer release()
			// Nine tests pre-mutation admission. Two tests cleanup of another admitted
			// move while its physical provider COPY is still in flight.
			blockedUID := uint32(100 + count - 1)
			f.serverA.sess.copyBlock = map[uint32]<-chan struct{}{blockedUID: gate}
			ids := make([]string, 0, count)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("bulk-%02d", i)
				uid := uint32(100 + i)
				msgID := ""
				if i == count-1 {
					msgID = "<preserve@example.test>"
				}
				if err := f.app.messageStore.Create(&message.Message{ID: id, AccountID: "acct-a", FolderID: "a-inbox", UID: uid, MessageID: msgID, FromEmail: "bulk@example.test", Subject: "bulk", Date: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
				f.serverA.sess.mu.Lock()
				f.serverA.sess.messages[uid] = []byte(fmt.Sprintf("From: bulk@example.test\r\nTo: me@example.test\r\nSubject: bulk-%02d\r\nMessage-ID: %s\r\n\r\nbody", i, msgID))
				f.serverA.sess.mu.Unlock()
				ids = append(ids, id)
			}
			last := ids[len(ids)-1]
			if err := f.app.messageStore.MarkReadReceiptHandled(last); err != nil {
				t.Fatal(err)
			}
			rule := &mailfilter.Rule{ID: "bulk", Name: "Bulk", AccountIDs: []string{"acct-a"}, FromContains: []string{"bulk@example.test"}, DestFolderID: "a-trash"}
			if err := f.app.filterStore.Create(rule); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := f.app.ApplyFilterRuleToExisting(rule.ID); done <- err }()
			// Wait for at least one actual provider COPY, then let its fallback sync
			// finish while the final COPY is blocked (or still awaiting admission).
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("no provider COPY")
			}
			waitFor(t, 5*time.Second, func() bool {
				rows, err := f.app.messageStore.GetByIDs([]string{ids[0]})
				if err != nil || len(rows) != 1 || int32(rows[0].UID) >= 0 {
					return false
				}
				var copies int
				err = f.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE folder_id='a-trash' AND uid>0 AND subject='bulk-00'`).Scan(&copies)
				return err == nil && copies == 1
			})
			rows, err := f.app.messageStore.GetByIDs([]string{last})
			if err != nil || len(rows) != 1 {
				t.Fatal("fallback erased another move's row", err, len(rows))
			}
			var handled bool
			if err := f.db.QueryRow(`SELECT read_receipt_handled FROM messages WHERE id=?`, last).Scan(&handled); err != nil || !handled {
				t.Fatal("read receipt dismissal lost", err)
			}
			release()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("bulk run did not finish")
			}
			releaseBoundary := f.app.acquireFilterExecutionBoundary()
			releaseBoundary()
			var actualCount int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE folder_id='a-trash' AND uid>0 AND (subject LIKE 'bulk-%' OR id=?)`, last).Scan(&actualCount); err != nil || actualCount != count {
				t.Fatal("completed copies not rediscovered", actualCount, count, err)
			}
			rows, err = f.app.messageStore.GetByIDs([]string{last})
			if err != nil || len(rows) != 1 || rows[0].FolderID != "a-trash" || rows[0].UID == 0 {
				t.Fatal("completed copy lost original identity", rows, err)
			}
			if err := f.db.QueryRow(`SELECT read_receipt_handled FROM messages WHERE id=? AND uid>0`, last).Scan(&handled); err != nil || !handled {
				t.Fatal("rediscovery lost dismissal", err)
			}
		})
	}
}

func TestFullFilterMoveLimitDoesNotMutateNextMessage(t *testing.T) {
	f := newFilterFixture(t, false)
	holds := make([]*undo.MoveCompletion, 8)
	for i := range holds {
		holds[i] = undo.NewMoveCompletion()
		f.app.trackFilterMove(holds[i])
	}
	defer func() {
		for _, h := range holds {
			h.Complete(nil)
		}
	}()
	rule := *f.rule
	rule.DestFolderID = "a-trash"
	done := make(chan struct{})
	go func() { _, _ = f.app.applyFilterActions(&rule, []string{f.message.ID}, f.message); close(done) }()
	select {
	case <-done:
		t.Fatal("admitted beyond bound")
	case <-time.After(30 * time.Millisecond):
	}
	if got := f.messageFolderID(t); got != "a-inbox" {
		t.Fatal("optimistic write occurred before slot admission", got)
	}
	for _, h := range holds {
		h.Complete(nil)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("admission never resumed")
	}
	releaseBoundary := f.app.acquireFilterExecutionBoundary()
	releaseBoundary()
}

func TestFilterMoveSlotSurvivesPanicAndMultipleCompletions(t *testing.T) {
	a := &App{}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		_ = a.withFilterMoveSlot(func(func(*undo.MoveCompletion)) error { panic("mutation") })
	}()
	if len(a.filterMoveSlots) != 0 {
		t.Fatal("panic leaked reservation")
	}
	one, two := undo.NewMoveCompletion(), undo.NewMoveCompletion()
	if err := a.withFilterMoveSlot(func(bind func(*undo.MoveCompletion)) error { bind(one); bind(two); return nil }); err != nil {
		t.Fatal(err)
	}
	one.Complete(nil)
	time.Sleep(10 * time.Millisecond)
	if len(a.filterMoveSlots) != 1 {
		t.Fatal("first completion released shared reservation")
	}
	two.Complete(nil)
	deadline := time.Now().Add(time.Second)
	for len(a.filterMoveSlots) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("last completion leaked reservation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestShutdownWhileFilterWaitsForAdmissionDoesNotMutate(t *testing.T) {
	f := newFilterFixture(t, false)
	holds := make([]*undo.MoveCompletion, 8)
	for i := range holds {
		holds[i] = undo.NewMoveCompletion()
		f.app.trackFilterMove(holds[i])
	}
	defer func() {
		for _, h := range holds {
			h.Complete(nil)
		}
	}()
	rule := *f.rule
	rule.DestFolderID = "a-trash"
	done := make(chan error, 1)
	go func() { _, err := f.app.applyFilterActions(&rule, []string{f.message.ID}, f.message); done <- err }()
	select {
	case <-done:
		t.Fatal("expected admission wait")
	case <-time.After(30 * time.Millisecond):
	}
	f.app.syncOwnerMu.Lock()
	f.app.syncShutdown = true
	f.app.syncOwnerMu.Unlock()
	for _, h := range holds {
		h.Complete(nil)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("shutdown did not stop waiting mutation")
		}
	case <-time.After(time.Second):
		t.Fatal("admission did not wake")
	}
	if got := f.messageFolderID(t); got != "a-inbox" {
		t.Fatal("shutdown admitted local mutation", got)
	}
}

func TestAutomaticFilterRechecksRuleAfterAdmission(t *testing.T) {
	for _, change := range []string{"disable", "edit", "delete"} {
		t.Run(change, func(t *testing.T) {
			f := newFilterFixture(t, false)
			rule, err := f.app.filterStore.Get(f.rule.ID)
			if err != nil {
				t.Fatal(err)
			}
			holds := make([]*undo.MoveCompletion, 8)
			for i := range holds {
				holds[i] = undo.NewMoveCompletion()
				f.app.trackFilterMove(holds[i])
			}
			defer func() {
				for _, h := range holds {
					h.Complete(nil)
				}
			}()
			done := make(chan struct{})
			go func() { f.app.applyMatchedFilterRule(rule, f.message); close(done) }()
			select {
			case <-done:
				t.Fatal("expected admission wait")
			case <-time.After(30 * time.Millisecond):
			}
			switch change {
			case "delete":
				err = f.app.filterStore.Delete(rule.ID)
			default:
				live, e := f.app.filterStore.Get(rule.ID)
				if e != nil {
					t.Fatal(e)
				}
				if change == "disable" {
					live.Enabled = false
				} else {
					live.DestFolderID = "a-trash"
				}
				err = f.app.filterStore.Update(live)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range holds {
				h.Complete(nil)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("callback did not finish")
			}
			release := f.app.acquireFilterExecutionBoundary()
			release()
			if got := f.messageFolderID(t); got != "a-inbox" {
				t.Fatal("stale automatic rule moved mail", got)
			}
		})
	}
}

func TestCreateFilterRuleAppendsAfterSparsePriority(t *testing.T) {
	f := newFilterFixture(t, false)
	existing, err := f.app.filterStore.Get(f.rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	existing.OrderIndex = 50
	if err := f.app.filterStore.Update(existing); err != nil {
		t.Fatal(err)
	}
	next := *existing
	next.ID = "new-last"
	next.Name = "A new last rule"
	next.OrderIndex = 0
	if err := f.app.CreateFilterRule(&next); err != nil {
		t.Fatal(err)
	}
	rules, err := f.app.filterStore.List()
	if err != nil || rules[len(rules)-1].ID != next.ID || next.OrderIndex != 51 {
		t.Fatal("new rule jumped ahead", next.OrderIndex, rules, err)
	}
}

func TestInterruptedMoveRecoveryRequiresServerEvidenceAndRetainsLocalState(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.rediscoverCopies = true
	if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.MarkReadReceiptHandled(f.message.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.app.captureInterruptedMoveRecovery(); err != nil {
		t.Fatal(err)
	}

	if err := f.app.messageStore.Create(&message.Message{ID: "preexisting", AccountID: "acct-a", FolderID: "a-trash", UID: 1000, MessageID: f.message.MessageID, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[1000] = []byte(cannedRaw)
	f.serverA.sess.copiedUIDs = map[uint32]string{1000: "Trash"}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err == nil {
		t.Fatal("sole preexisting copy was guessed as move result")
	}
	if got, err := f.app.messageStore.Get("preexisting"); err != nil || got == nil {
		t.Fatal("preexisting copy overwritten", err)
	}
	if err := f.app.messageStore.RecordMoveUIDEvidence("a-trash", 99, map[string]uint32{f.message.ID: 1001}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err == nil {
		t.Fatal("changed UID validity accepted")
	}
	if err := f.app.messageStore.RecordMoveUIDEvidence("a-trash", 1, map[string]uint32{f.message.ID: 1001}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err == nil {
		t.Fatal("absent verified UID accepted")
	}
	if err := f.app.messageStore.Create(&message.Message{ID: "verified", AccountID: "acct-a", FolderID: "a-trash", UID: 1001, MessageID: f.message.MessageID, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[1001] = []byte(cannedRaw)
	f.serverA.sess.copiedUIDs[1001] = "Trash"
	f.serverA.sess.searchError = true
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err == nil {
		t.Fatal("failed verification accepted")
	}
	f.serverA.sess.searchError = false
	if err := f.app.messageStore.Create(&message.Message{ID: "live", AccountID: "acct-a", FolderID: "a-inbox", UID: 99, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.MoveMessages([]string{"live"}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := f.app.messageStore.Get(f.message.ID)
	if err != nil || got == nil || got.UID != 1001 {
		t.Fatal("verified identity not recovered", got, err)
	}
	var handled bool
	if err := f.db.QueryRow(`SELECT read_receipt_handled FROM messages WHERE id=?`, f.message.ID).Scan(&handled); err != nil || !handled {
		t.Fatal("dismissed receipt lost", err)
	}
	live, err := f.app.messageStore.Get("live")
	if err != nil || live == nil || int32(live.UID) >= 0 {
		t.Fatal("live move touched", live, err)
	}
	if got, err := f.app.messageStore.Get("preexisting"); err != nil || got == nil {
		t.Fatal("preexisting copy overwritten", err)
	}
}
func TestUnmatchedInterruptedMoveRetainsReceiptAfterOrdinarySync(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.rediscoverCopies = true
	if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.MarkReadReceiptHandled(f.message.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.app.captureInterruptedMoveRecovery(); err != nil {
		t.Fatal(err)
	}
	if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
		t.Fatal("unresolved move blocked routine refresh", err)
	}
	f.app.moveSyncMu.Lock()
	pendingCount := len(f.app.moveRecoveryIDs["a-trash"])
	f.app.moveSyncMu.Unlock()
	if pendingCount != 1 {
		t.Fatal("unresolved move lost retry status", pendingCount)
	}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err == nil {
		t.Fatal("move-specific recovery hid incomplete outcome")
	}
	got, err := f.app.messageStore.Get(f.message.ID)
	if err != nil || got == nil || int32(got.UID) >= 0 {
		t.Fatal("unmatched row lost", got, err)
	}
	var handled bool
	if err := f.db.QueryRow(`SELECT read_receipt_handled FROM messages WHERE id=?`, f.message.ID).Scan(&handled); err != nil || !handled {
		t.Fatal("dismissed receipt lost", err)
	}
}

func TestRecoveryDoesNotImportOrFilterHistoricalMail(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.rediscoverCopies = true
	f.serverA.sess.internalDate = time.Now().AddDate(0, -3, 0)
	if _, err := f.db.Exec(`UPDATE accounts SET sync_period_days=30 WHERE id='acct-a'`); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE messages SET date=? WHERE id=?`, time.Now().AddDate(0, -3, 0), f.message.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.RecordMoveUIDEvidence("a-trash", 1, map[string]uint32{f.message.ID: 1001}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.captureInterruptedMoveRecovery(); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[1001] = []byte(cannedRaw)
	f.serverA.sess.messages[1002] = []byte(strings.ReplaceAll(cannedRaw, "m1@example.com", "unrelated@example.com"))
	f.serverA.sess.copiedUIDs = map[uint32]string{1001: "Trash", 1002: "Trash"}
	hooks := make(chan string, 4)
	f.app.syncEngine.SetNewMessageHandler(func(_, _ string, m *message.Message) { hooks <- m.ID })
	if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
		t.Fatal(err)
	}
	got, err := f.app.messageStore.Get(f.message.ID)
	if err != nil || got == nil || got.UID != 1001 {
		t.Fatal("old pending move pruned", got, err)
	}
	select {
	case id := <-hooks:
		t.Fatal("historical recovery dispatched automatic filter", id)
	case <-time.After(50 * time.Millisecond):
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE folder_id='a-trash' AND uid=1002`).Scan(&n); err != nil || n != 0 {
		t.Fatal("recovery imported unrelated old mail", n, err)
	}
}
func TestEmptyMailboxSafeguardDoesNotAbortRoutineSync(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.rediscoverCopies = true
	if _, err := f.db.Exec(`UPDATE accounts SET sync_period_days=0 WHERE id='acct-a'`); err != nil {
		t.Fatal(err)
	}
	if err := f.app.SyncFolder("acct-a", "a-inbox"); err != nil {
		t.Fatal("empty listing aborted routine sync", err)
	}
	if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
		t.Fatal("next folder sync blocked", err)
	}
	if got, err := f.app.messageStore.Get(f.message.ID); err != nil || got == nil {
		t.Fatal("empty-list safety lost cache", err)
	}
}

func (c *testIMAPConnection) Store(w *imapserver.FetchWriter, n imap.NumSet, flags *imap.StoreFlags, o *imap.StoreOptions) error {
	c.mu.Lock()
	enabled := c.deleteSource
	c.mu.Unlock()
	if enabled {
		for _, f := range flags.Flags {
			if f == imap.FlagDeleted {
				if uids, ok := n.(imap.UIDSet); ok {
					nums, _ := uids.Nums()
					for _, uid := range nums {
						c.deleted = append(c.deleted, uint32(uid))
					}
				}
			}
		}
	}
	return c.testIMAPSession.Store(w, n, flags, o)
}
func (c *testIMAPConnection) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	c.mu.Lock()
	enabled := c.deleteSource
	callback := c.afterExpunge
	if enabled {
		for _, uid := range c.deleted {
			delete(c.messages, uid)
			delete(c.copiedUIDs, uid)
		}
		c.deleted = nil
	}
	c.mu.Unlock()
	if enabled && callback != nil && callback() {
		_ = c.conn.Close()
	}
	return nil
}
func TestMoveCopyProofSurvivesPostExpungeDisconnectWithoutReplay(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprint(reset), func(t *testing.T) {
			f := newFilterFixture(t, false)
			f.app.ctx = context.Background()
			f.serverA.sess.rediscoverCopies = true
			f.serverA.sess.deleteSource = true
			f.serverA.sess.copiedUIDs = map[uint32]string{f.message.UID: "INBOX"}
			if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
				t.Fatal(err)
			}
			var callbackOnce stdsync.Once
			f.serverA.sess.afterExpunge = func() bool {
				closeConn := false
				callbackOnce.Do(func() {
					uid, v, err := f.app.messageStore.MoveUIDEvidence("a-trash", f.message.ID)
					if err != nil || uid == 0 || v != 1 {
						t.Error("COPYUID was not durable before cleanup", uid, v, err)
					}
					_, _, _, done, err := f.app.messageStore.MoveCleanupState("a-trash", f.message.ID)
					if err != nil || done {
						t.Error("copy was reported as completed move", done, err)
					}
					if reset {
						f.serverA.sess.mu.Lock()
						f.serverA.sess.validity = 2
						f.serverA.sess.mu.Unlock()
					}
					closeConn = true
				})
				return closeConn
			}
			dest, err := f.app.folderStore.Get("a-trash")
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.app.moveMessagesToIMAP([]*message.Message{f.message}, "a-inbox", dest, ""); err == nil {
				t.Fatal("disconnect did not interrupt local completion")
			}
			f.serverA.sess.mu.Lock()
			copies := f.serverA.sess.copyCount
			f.serverA.sess.mu.Unlock()
			if copies != 1 {
				t.Fatal("transport retry replayed COPY", copies)
			}
			if uid, v, err := f.app.messageStore.MoveUIDEvidence("a-trash", f.message.ID); err != nil || uid == 0 || v != 1 {
				t.Fatal("retry relabelled old proof", uid, v, err)
			}
			err = f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID})
			if reset {
				if err == nil {
					t.Fatal("new generation accepted old copy proof")
				}
				return
			}
			if err != nil {
				t.Fatal("confirmed move not recovered after interrupted expunge response", err)
			}
			got, err := f.app.messageStore.Get(f.message.ID)
			if err != nil || got == nil || int32(got.UID) < 0 {
				t.Fatal("recovery did not restore stable identity", got, err)
			}
		})
	}
}
func TestConfirmedCopyWithSourcePresentIsNotCompletedMove(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.rediscoverCopies = true
	if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.RecordCopiedMoveUIDs("a-trash", 1, map[string]uint32{f.message.ID: 701}, "a-inbox", 1, map[string]uint32{f.message.ID: f.message.UID}); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[701] = []byte(cannedRaw)
	f.serverA.sess.copiedUIDs = map[uint32]string{701: "Trash", f.message.UID: "INBOX"}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID}); err == nil {
		t.Fatal("incomplete source cleanup was treated as successful move")
	}
	if got, err := f.app.messageStore.Get(f.message.ID); err != nil || got == nil || int32(got.UID) >= 0 {
		t.Fatal("incomplete move reconciled", got, err)
	}
	if err := f.app.messageStore.Create(&message.Message{ID: "completed-after-unresolved", AccountID: "acct-a", FolderID: "a-inbox", UID: 88, MessageID: f.message.MessageID, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.MoveMessages([]string{"completed-after-unresolved"}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[702] = []byte(cannedRaw)
	f.serverA.sess.copiedUIDs[702] = "Trash"
	if err := f.app.messageStore.RecordMoveUIDEvidence("a-trash", 1, map[string]uint32{"completed-after-unresolved": 702}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.recoverInterruptedMoves(context.Background(), "acct-a", "a-trash", []string{f.message.ID, "completed-after-unresolved"}); err == nil {
		t.Fatal("first incomplete outcome was hidden")
	}
	if got, err := f.app.messageStore.Get("completed-after-unresolved"); err != nil || got == nil || got.UID != 702 {
		t.Fatal("prior source selection prevented next recovery", got, err)
	}
}

func TestUIDResetQuarantinesRepairedMailBeforeReusingUID(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	oldUID := f.message.UID
	attachments := message.NewAttachmentStore(f.db)
	if _, err := attachments.RepairFromMIME("acct-a", f.message.ID, "snapshot", []message.RepairPart{{Metadata: message.Attachment{ID: "fixed", MessageID: f.message.ID, Filename: "invoice.txt", Size: 3}, Payload: []byte("one")}}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE folders SET uid_validity=1 WHERE id='a-inbox'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE accounts SET sync_period_days=0 WHERE id='acct-a'`); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.validity = 2
	f.serverA.sess.rediscoverCopies = true
	f.serverA.sess.messages[oldUID] = []byte(strings.ReplaceAll(cannedRaw, "m1@example.com", "unrelated@example.com"))
	f.serverA.sess.copiedUIDs = map[uint32]string{oldUID: "INBOX"}
	if err := f.app.SyncFolder("acct-a", "a-inbox"); err != nil {
		t.Fatal(err)
	}
	original, err := f.app.messageStore.Get(f.message.ID)
	if err != nil || original == nil || int32(original.UID) >= 0 || original.MessageID != f.message.MessageID {
		t.Fatal("repaired row retained addressable stale UID", original, err)
	}
	var remoteID, rfcID string
	if err := f.db.QueryRow(`SELECT id,message_id FROM messages WHERE folder_id='a-inbox' AND uid=?`, oldUID).Scan(&remoteID, &rfcID); err != nil || remoteID == f.message.ID || !strings.Contains(rfcID, "unrelated") {
		t.Fatal("reused UID was not fetched as distinct mail", remoteID, rfcID, err)
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM attachment_repair_backups WHERE message_id=?`, f.message.ID).Scan(&n); err != nil || n == 0 {
		t.Fatal("repair history lost", n, err)
	}
	if _, err := f.app.syncEngine.FetchMessageBody(context.Background(), "acct-a", f.message.ID); err == nil {
		t.Fatal("quarantined row was addressable for body fetch")
	}
	if err := f.app.MoveToFolder([]string{f.message.ID}, "a-trash"); err == nil {
		t.Fatal("quarantined row was addressable for move")
	}
	f.serverA.sess.mu.Lock()
	copies := f.serverA.sess.copyCount
	f.serverA.sess.mu.Unlock()
	if copies != 0 {
		t.Fatal("quarantined row targeted server mail", copies)
	}
}
func TestCOPYUIDUsesItsOwnGenerationNotPriorSelect(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.copyValidity = 7
	f.serverA.sess.rediscoverCopies = true
	if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	dest, err := f.app.folderStore.Get("a-trash")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.app.moveMessagesToIMAP([]*message.Message{f.message}, "a-inbox", dest, ""); err == nil {
		t.Fatal("generation mismatch was accepted")
	}
	if _, v, err := f.app.messageStore.MoveUIDEvidence("a-trash", f.message.ID); err != nil || v != 7 {
		t.Fatal("COPYUID generation was replaced by SELECT generation", v, err)
	}
}

func TestMoveWaitsForDestinationCacheResetBeforePromotion(t *testing.T) {
	for _, kind := range []string{"plain", "repaired", "no-message-id"} {
		t.Run(kind, func(t *testing.T) {
			f := newFilterFixture(t, false)
			if kind == "repaired" {
				attachments := message.NewAttachmentStore(f.db)
				if _, err := attachments.RepairFromMIME("acct-a", f.message.ID, "snapshot", []message.RepairPart{{Metadata: message.Attachment{ID: "fixed", MessageID: f.message.ID, Filename: "invoice.txt", Size: 3}, Payload: []byte("one")}}, ""); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "no-message-id" {
				if _, err := f.db.Exec(`UPDATE messages SET message_id='' WHERE id=?`, f.message.ID); err != nil {
					t.Fatal(err)
				}
				f.message.MessageID = ""
				f.serverA.sess.messages[f.message.UID] = []byte(strings.ReplaceAll(cannedRaw, "Message-ID: <m1@example.com>\r\n", ""))
			}
			f.app.ctx = context.Background()
			f.serverA.sess.rediscoverCopies = true
			f.serverA.sess.validity = 7
			f.serverA.sess.resetOnCopy = 8
			f.serverA.sess.deleteSource = true
			f.serverA.sess.copiedUIDs = map[uint32]string{f.message.UID: "INBOX"}
			if _, err := f.db.Exec(`UPDATE folders SET uid_validity=7 WHERE id IN ('a-inbox','a-trash')`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE accounts SET sync_period_days=0 WHERE id='acct-a'`); err != nil {
				t.Fatal(err)
			}
			f.serverA.sess.messages[f.message.UID] = bytes.Replace(f.serverA.sess.messages[f.message.UID], []byte("Subject:"), []byte("Disposition-Notification-To: bob@example.com\r\nSubject:"), 1)
			if err := f.app.messageStore.MarkReadReceiptHandled(f.message.ID); err != nil {
				t.Fatal(err)
			}
			if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
				t.Fatal(err)
			}
			dest, err := f.app.folderStore.Get("a-trash")
			if err != nil {
				t.Fatal(err)
			}
			resolved, _, err := f.app.moveMessagesToIMAP([]*message.Message{f.message}, "a-inbox", dest, "")
			if err != nil || len(resolved) != 1 {
				t.Fatal("COPY and final SELECT should agree in new generation", resolved, err)
			}
			if _, err := f.app.reconcileMovedMessageUIDsWithRetry("a-trash", resolved); err == nil {
				t.Fatal("copy promoted into stale generation cache")
			}
			pending, err := f.app.messageStore.Get(f.message.ID)
			if err != nil || pending == nil || int32(pending.UID) >= 0 {
				t.Fatal("pending identity was not retained", pending, err)
			}
			f.app.retainMoveRecovery("a-trash", []string{f.message.ID})
			if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
				t.Fatal(err)
			}
			got, err := f.app.messageStore.Get(f.message.ID)
			if err != nil || got == nil || got.UID != resolved[f.message.ID] {
				t.Fatal("reset did not recover original identity", got, err)
			}
			var handled bool
			if err := f.db.QueryRow(`SELECT read_receipt_handled FROM messages WHERE id=?`, f.message.ID).Scan(&handled); err != nil || !handled {
				t.Fatal("receipt decision lost across cache generation", err)
			}
			f.app.syncMu.Lock()
			delete(f.app.syncLastRequest, "acct-a:a-trash")
			f.app.syncMu.Unlock()
			if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(`SELECT read_receipt_handled FROM messages WHERE id=?`, f.message.ID).Scan(&handled); err != nil || !handled {
				t.Fatal("header backfill erased receipt decision", err)
			}
			if kind == "repaired" {
				if _, err := f.db.Exec(`UPDATE accounts SET sync_period_days=30 WHERE id='acct-a'`); err != nil {
					t.Fatal(err)
				}
				f.serverA.sess.internalDate = time.Now().AddDate(0, -3, 0)
				if _, err := f.db.Exec(`UPDATE messages SET date=? WHERE id=?`, f.serverA.sess.internalDate, f.message.ID); err != nil {
					t.Fatal(err)
				}
				f.app.syncMu.Lock()
				delete(f.app.syncLastRequest, "acct-a:a-trash")
				f.app.syncMu.Unlock()
				if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
					t.Fatal(err)
				}
				repaired, err := f.app.messageStore.Get(f.message.ID)
				if err != nil || repaired == nil || int32(repaired.UID) >= 0 {
					t.Fatal("date omission lost repair or retained addressable UID", repaired, err)
				}
				var backups int
				if err := f.db.QueryRow(`SELECT COUNT(*) FROM attachment_repair_backups WHERE message_id=?`, f.message.ID).Scan(&backups); err != nil || backups == 0 {
					t.Fatal("date omission destroyed repair history", backups, err)
				}
				var proofs int
				if err := f.db.QueryRow(`SELECT COUNT(*) FROM move_uid_evidence WHERE message_id=?`, f.message.ID).Scan(&proofs); err != nil || proofs != 0 {
					t.Fatal("retired snapshot retained recovery proof", proofs, err)
				}

			}

		})
	}
}
func TestConversationMoveSkipsOnlyQuarantinedRepairSnapshots(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	if _, err := f.db.Exec(`UPDATE messages SET thread_id='thread' WHERE id=?`, f.message.ID); err != nil {
		t.Fatal(err)
	}
	attachments := message.NewAttachmentStore(f.db)
	if _, err := attachments.RepairFromMIME("acct-a", f.message.ID, "snapshot", []message.RepairPart{{Metadata: message.Attachment{ID: "fixed", MessageID: f.message.ID, Filename: "invoice.txt", Size: 3}, Payload: []byte("one")}}, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.DeleteSyncedUIDCache("a-inbox"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.Create(&message.Message{ID: "live-thread-mail", AccountID: "acct-a", FolderID: "a-inbox", UID: 11, MessageID: "live@test", ThreadID: "thread", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[11] = []byte(strings.ReplaceAll(cannedRaw, "m1@example.com", "live@test"))
	f.app.moveFilterHistory = mailfilter.NewMoveHistory(f.db)
	if err := f.app.runUndoMoveMutation([]string{f.message.ID, "live-thread-mail"}, "a-trash", "old-undo"); err == nil {
		t.Fatal("undo silently skipped quarantined snapshot")
	}
	before, err := f.app.messageStore.Get("live-thread-mail")
	if err != nil || before == nil || before.FolderID != "a-inbox" {
		t.Fatal("failed undo partially moved selection", before, err)
	}
	if _, err := f.app.MoveToFolderWithUndo([]string{f.message.ID, "live-thread-mail"}, "a-trash"); err != nil {
		t.Fatal("quarantined snapshot blocked live conversation move", err)
	}
	var historyCount int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM mail_filter_move_history`).Scan(&historyCount); err != nil || historyCount != 1 {
		t.Fatal("move history included skipped snapshot", historyCount, err)
	}
	release := f.app.acquireFilterExecutionBoundary()
	release()
	snapshot, err := f.app.messageStore.Get(f.message.ID)
	if err != nil || snapshot == nil || snapshot.FolderID != "a-inbox" || int32(snapshot.UID) >= 0 {
		t.Fatal("snapshot moved", snapshot, err)
	}
	live, err := f.app.messageStore.Get("live-thread-mail")
	if err != nil || live == nil || live.FolderID != "a-trash" {
		t.Fatal("live conversation mail did not move", live, err)
	}
	if err := f.app.messageStore.Create(&message.Message{ID: "unresolved", AccountID: "acct-a", FolderID: "a-inbox", UID: 12, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.MoveMessages([]string{"unresolved"}, "a-inbox"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.MoveToFolder([]string{f.message.ID, "unresolved"}, "a-trash"); err == nil {
		t.Fatal("unresolved optimistic move guard was bypassed")
	}
}

func TestUnresolvedMoveDoesNotDropClassificationBackfill(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.serverA.sess.rediscoverCopies = true
	if err := f.app.messageStore.MoveMessages([]string{f.message.ID}, "a-trash"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.RecordMoveUIDEvidence("a-trash", 99, map[string]uint32{f.message.ID: 701}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.Create(&message.Message{ID: "uncategorized", AccountID: "acct-a", FolderID: "a-trash", UID: 702, MessageID: "classify@test", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[702] = []byte(strings.ReplaceAll(cannedRaw, "m1@example.com", "classify@test"))
	f.serverA.sess.copiedUIDs = map[uint32]string{702: "Trash"}
	if err := f.app.SyncFolder("acct-a", "a-trash"); err != nil {
		t.Fatal(err)
	}
	var category string
	if err := f.db.QueryRow(`SELECT inbox_category FROM messages WHERE id='uncategorized'`).Scan(&category); err != nil || category == "" {
		t.Fatal("unresolved proof stalled classification", category, err)
	}
}

func TestForwardMovePartitionWithSharedOperationSkipsSnapshot(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	if _, err := f.db.Exec(`UPDATE messages SET uid=-rowid WHERE id=?`, f.message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO quarantined_repair_messages(message_id) VALUES(?)`, f.message.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.app.messageStore.Create(&message.Message{ID: "live", AccountID: "acct-a", FolderID: "a-inbox", UID: 11, MessageID: "live@test", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.serverA.sess.messages[11] = []byte(strings.ReplaceAll(cannedRaw, "m1@example.com", "live@test"))
	if _, err := f.app.moveToFolderWithService([]string{f.message.ID, "live"}, "a-trash", true, "shared-forward-operation", false); err != nil {
		t.Fatal("forward shared operation was mistaken for undo", err)
	}
	waitFor(t, time.Second, func() bool {
		f.serverA.sess.mu.Lock()
		defer f.serverA.sess.mu.Unlock()
		return f.serverA.sess.copyCount == 1
	})
	got, err := f.app.messageStore.Get("live")
	if err != nil || got == nil || got.FolderID != "a-trash" {
		t.Fatal("live forward partition was skipped", got, err)
	}
}
