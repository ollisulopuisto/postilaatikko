package app

import (
	"context"
	"errors"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"path/filepath"
	"testing"
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
