package alwayson

import (
	"context"
	"errors"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/scheduledsend"
	"path/filepath"
	"testing"
)

func TestSendPreflightRejectionIsDefinitive(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	r := &SyncRuntime{db: db, immediateSend: func(context.Context, scheduledsend.Delivery) error { t.Fatal("sent rejected message"); return nil }, validateSender: func(string, string) error { return errors.New("sender unavailable") }}
	req := v1.SendRequest{AccountID: "a", From: "a@example.test", To: []string{"b@example.test"}, RFC822: []byte("From: a@example.test\r\nTo: b@example.test\r\nSubject: test\r\n\r\nbody")}
	_, err = r.Send(context.Background(), req, "request", "hash")
	if !v1.IsCode(err, v1.CodeInvalidRequest) {
		t.Fatalf("preflight not classified definitive: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM send_api_idempotency`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reserved before rejection count=%d err=%v", count, err)
	}
}
