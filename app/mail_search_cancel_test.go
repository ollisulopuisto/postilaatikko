package app

import (
	"context"
	"errors"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	syncengine "github.com/hkdb/aerion/internal/sync"
	"path/filepath"
	"testing"
	"time"
)

func TestCanceledMailSearchReturnsCancellationInsteadOfAnAccountIssue(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &App{ctx: ctx, accountStore: account.NewStore(db)}
	page, err := a.SearchMail(MailSearchRequest{SessionID: "superseded"})
	if !errors.Is(err, context.Canceled) || page != nil {
		t.Fatalf("canceled search returned page=%+v err=%v; want cancellation, no account issues", page, err)
	}
}

func TestSupersedingMailSearchDoesNotReturnGmailCancellationIssue(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type,enabled) VALUES('a','A','a@example.org','imap.gmail.com','smtp.gmail.com','a','oauth2',1)`)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), accountStore: account.NewStore(db), folderStore: folder.NewStore(db), messageStore: message.NewStore(db)}
	a.syncEngine = syncengine.NewEngine(nil, a.accountStore, a.folderStore, a.messageStore, message.NewAttachmentStore(db))
	entered := make(chan struct{})
	a.syncEngine.SetGmailTokenSource(func(ctx context.Context, _ string, _ bool) (string, error) {
		select {
		case entered <- struct{}{}:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		<-ctx.Done()
		return "", ctx.Err()
	})
	type reply struct {
		page *MailSearchPage
		err  error
	}
	old := make(chan reply, 1)
	go func() { page, err := a.SearchMail(MailSearchRequest{SessionID: "same"}); old <- reply{page, err} }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old search did not start")
	}
	newer := make(chan reply, 1)
	go func() { page, err := a.SearchMail(MailSearchRequest{SessionID: "same"}); newer <- reply{page, err} }()
	select {
	case got := <-old:
		if !errors.Is(got.err, context.Canceled) || got.page != nil {
			t.Fatalf("superseded search returned %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("superseded search did not stop")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("new search did not start")
	}
	a.CancelMailSearch("same")
	select {
	case got := <-newer:
		if !errors.Is(got.err, context.Canceled) || got.page != nil {
			t.Fatalf("canceled replacement returned %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement search did not stop")
	}
}
