package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

// A virtual-folder view must keep loading when the mailbox service's cached
// view hits transient SQLite lock contention (SQLITE_BUSY): the conversations
// are readable without the write-side cache, so the view falls back to the
// direct local read instead of failing the whole list.
func TestGetVirtualFolderCachedViewFallsBackOnBusyHelperCache(t *testing.T) {
	db := openMailboxMutationTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username)
		 VALUES ('acc', 'Account', 'a@example.com', 'imap.example.com', 'smtp.example.com', 'a')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('fld', 'acc', 'INBOX', 'INBOX', 'inbox')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	msgStore := message.NewStore(db)
	if err := msgStore.Create(&message.Message{
		ID: "m1", AccountID: "acc", FolderID: "fld", UID: 1,
		Subject: "unread one", ThreadID: "m1", IsRead: false,
		Date: time.Now(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	vfStore := virtualfolder.NewStore(db)
	vf := &virtualfolder.VirtualFolder{
		ID: "vf-unread", Name: "lukematta", ReadStatus: "unread",
		FolderIDs: []string{"fld"},
	}
	if err := vfStore.Create(vf); err != nil {
		t.Fatalf("create virtual folder: %v", err)
	}

	connector := &mailboxServiceConnector{
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true}
		},
		cachedViewFunc: func(context.Context, string) (*v1.CachedVirtualFolderView, error) {
			return nil, errors.New("failed to begin transaction: database is locked (5) (SQLITE_BUSY)")
		},
	}
	a := &App{
		ctx:                context.Background(),
		mailboxService:     connector,
		virtualFolderStore: vfStore,
		messageStore:       msgStore,
	}

	view, err := a.GetVirtualFolderCachedView(vf.ID)
	if err != nil {
		t.Fatalf("GetVirtualFolderCachedView: %v (want direct-read fallback, not the busy error)", err)
	}
	if view == nil || len(view.Conversations) != 1 || view.Conversations[0].ThreadID != "m1" {
		t.Fatalf("view = %#v, want one conversation m1 from the direct read", view)
	}
	if view.HasCache {
		t.Fatalf("HasCache = true, want false for a direct-read fallback")
	}
}

// The helper reports contention as its public database_busy code (the raw
// SQLite text never crosses the API); the desktop must still fall back.
func TestGetVirtualFolderCachedViewFallsBackOnBusyCode(t *testing.T) {
	db, a := busyFallbackTestApp(t, errors.New("api error (database_busy): Mailbox database is busy"))
	view, err := a.GetVirtualFolderCachedView("vf-unread")
	if err != nil {
		t.Fatalf("GetVirtualFolderCachedView: %v (want direct-read fallback for database_busy)", err)
	}
	if view == nil || len(view.Conversations) != 1 || view.HasCache {
		t.Fatalf("view = %#v, want one direct-read conversation, HasCache=false", view)
	}
	_ = db
}

// The desktop's two-second request budget expires long before the helper's
// lock wait ends; a cache-route timeout gets the same direct-read fallback.
func TestGetVirtualFolderCachedViewFallsBackOnCacheTimeout(t *testing.T) {
	_, a := busyFallbackTestApp(t, fmt.Errorf("execute request: %w", context.DeadlineExceeded))
	view, err := a.GetVirtualFolderCachedView("vf-unread")
	if err != nil {
		t.Fatalf("GetVirtualFolderCachedView: %v (want direct-read fallback on cache timeout)", err)
	}
	if view == nil || len(view.Conversations) != 1 || view.HasCache {
		t.Fatalf("view = %#v, want one direct-read conversation, HasCache=false", view)
	}
}

func busyFallbackTestApp(t *testing.T, helperErr error) (*database.DB, *App) {
	t.Helper()
	db := openMailboxMutationTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username)
		 VALUES ('acc', 'Account', 'a@example.com', 'imap.example.com', 'smtp.example.com', 'a')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('fld', 'acc', 'INBOX', 'INBOX', 'inbox')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	msgStore := message.NewStore(db)
	if err := msgStore.Create(&message.Message{
		ID: "m1", AccountID: "acc", FolderID: "fld", UID: 1,
		Subject: "unread one", ThreadID: "m1", IsRead: false,
		Date: time.Now(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	vfStore := virtualfolder.NewStore(db)
	if err := vfStore.Create(&virtualfolder.VirtualFolder{
		ID: "vf-unread", Name: "lukematta", ReadStatus: "unread",
		FolderIDs: []string{"fld"},
	}); err != nil {
		t.Fatalf("create virtual folder: %v", err)
	}
	connector := &mailboxServiceConnector{
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true}
		},
		cachedViewFunc: func(context.Context, string) (*v1.CachedVirtualFolderView, error) {
			return nil, helperErr
		},
	}
	return db, &App{
		ctx:                context.Background(),
		mailboxService:     connector,
		virtualFolderStore: vfStore,
		messageStore:       msgStore,
	}
}

// Non-busy helper failures must keep failing closed: they are not contention
// and hiding them behind a local read would mask real service problems.
func TestGetVirtualFolderCachedViewKeepsFailingOnNonBusyHelperError(t *testing.T) {
	db := openMailboxMutationTestDB(t)
	connector := &mailboxServiceConnector{
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true}
		},
		cachedViewFunc: func(context.Context, string) (*v1.CachedVirtualFolderView, error) {
			return nil, errors.New("helper schema mismatch")
		},
	}
	a := &App{
		ctx:                context.Background(),
		mailboxService:     connector,
		virtualFolderStore: virtualfolder.NewStore(db),
	}
	if _, err := a.GetVirtualFolderCachedView("vf-unread"); err == nil {
		t.Fatal("GetVirtualFolderCachedView succeeded on a non-busy helper error")
	}
}
