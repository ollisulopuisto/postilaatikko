package pendingremote

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestQueueRejectsDuplicateDraftAndOversize(t *testing.T) {
	s := testStore(t)
	item := Item{ID: "one", AccountID: "a", DraftID: "draft", Payload: "{}", Prepared: []byte("mime"), DispatchAt: time.Now().Add(30 * time.Second)}
	if err := s.Queue(item); err != nil {
		t.Fatal(err)
	}
	item.ID = "two"
	if err := s.Queue(item); err == nil {
		t.Fatal("second queue for same account/draft accepted")
	}
	item.AccountID = "b"
	if err := s.Queue(item); err != nil {
		t.Fatal(err)
	}
	item.ID = "huge"
	item.DraftID = "huge"
	item.Prepared = make([]byte, MaxPreparedBytes+1)
	if err := s.Queue(item); err == nil {
		t.Fatal("unbounded MIME accepted")
	}
}
func TestCancelAndClaimHaveSingleWinner(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 20; i++ {
		id := time.Now().String()
		if err := s.Queue(Item{ID: id, AccountID: "a", Payload: "{}", Prepared: []byte("mime"), DispatchAt: time.Now().Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		wins := make(chan string, 2)
		go func() {
			defer wg.Done()
			if err := s.Cancel(id); err == nil {
				wins <- "cancel"
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Claim(id); err == nil {
				wins <- "claim"
			}
		}()
		wg.Wait()
		close(wins)
		n := 0
		for winner := range wins {
			n++
			if winner == "claim" {
				if err := s.Finish(id, StateSent); err != nil {
					t.Fatal(err)
				}
			}
		}
		if n != 1 {
			t.Fatalf("winners=%d", n)
		}
	}
}
func TestClaimIsImmutableAndCannotBeCompletedAsPending(t *testing.T) {
	s := testStore(t)
	original := []byte("immutable MIME")
	if err := s.Queue(Item{ID: "one", AccountID: "a", Payload: "{}", Prepared: original, DispatchAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Claim("one")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Prepared) != string(original) {
		t.Fatal("prepared bytes changed")
	}
	if _, err := s.Claim("one"); err == nil {
		t.Fatal("duplicate dispatch claimed")
	}
	if err := s.Cancel("one"); err == nil {
		t.Fatal("cancelled after claim")
	}
	if err := s.Finish("one", StateUnknown); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim("one"); err == nil {
		t.Fatal("unknown outcome claimed for resend")
	}
}
func TestRestartExpiresOnlyUnsubmittedRows(t *testing.T) {
	s := testStore(t)
	for _, id := range []string{"pending", "sending"} {
		if err := s.Queue(Item{ID: id, AccountID: "a", Payload: "{}", Prepared: []byte("mime"), DispatchAt: time.Now().Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Claim("sending"); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	pending, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatal("restart auto-recovered rows for delivery")
	}
	var state string
	if err := s.db.QueryRow(`SELECT state FROM remote_pending_sends WHERE id='sending'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StateUnknown {
		t.Fatalf("in-flight restart state=%s", state)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COALESCE(sum(length(prepared)),0) FROM remote_pending_sends WHERE id='pending'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("expired body retained")
	}
}

func TestQuotaCountsUnknownAndPayloadBytes(t *testing.T) {
	s := testStore(t)
	for i := 0; i < MaxRows; i++ {
		id := fmt.Sprintf("item-%d", i)
		if err := s.Queue(Item{ID: id, AccountID: "a", Payload: "{}", Prepared: []byte("mime"), DispatchAt: time.Now().Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Claim(id); err != nil {
			t.Fatal(err)
		}
		if err := s.Finish(id, StateUnknown); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Queue(Item{ID: "overflow", AccountID: "a", Payload: "{}", Prepared: []byte("mime"), DispatchAt: time.Now().Add(time.Minute)}); err == nil {
		t.Fatal("unknown outcomes bypassed quota")
	}
}
