package mailfilter

import (
	"testing"

	"github.com/hkdb/aerion/internal/message"
)

func moved(from, subject string) []*message.Message {
	return []*message.Message{{
		AccountID: "account1", FolderID: "folder1", FromEmail: from, Subject: subject,
		ToList: `[{"name":"","email":"me@example.com"}]`, CcList: "[]",
	}}
}

func TestMoveHistoryNeedsTwoIndependentActions(t *testing.T) {
	h := NewMoveHistory(setupTestDB(t))
	if err := h.Record("a1", "folder2", moved("news@shop.example", "Weekly invoice 1234")); err != nil {
		t.Fatal(err)
	}
	got, err := h.SuggestionForRecentMove("a1", "account1", "folder2")
	if err != nil || got != nil {
		t.Fatalf("one move must not suggest: %v %v", got, err)
	}
	if err := h.Record("a2", "folder2", moved("news@shop.example", "Another invoice 9876")); err != nil {
		t.Fatal(err)
	}
	got, err = h.SuggestionForRecentMove("a2", "account1", "folder2")
	if err != nil || got == nil {
		t.Fatalf("second matching move must suggest: %v %v", got, err)
	}
	if len(got.FromContains) != 1 || got.FromContains[0] != "news@shop.example" || got.SourceFolderID != "folder1" || got.DestFolderID != "folder2" {
		t.Fatalf("unexpected suggestion %+v", got)
	}
}

func TestMoveHistoryDismissedSuggestionIsNotOfferedAgain(t *testing.T) {
	h := NewMoveHistory(setupTestDB(t))
	_ = h.Record("a1", "folder2", moved("x@y.example", "alpha"))
	_ = h.Record("a2", "folder2", moved("x@y.example", "beta"))
	s, err := h.SuggestionForRecentMove("a2", "account1", "folder2")
	if err != nil || s == nil {
		t.Fatalf("want suggestion: %v %v", s, err)
	}
	if err := h.SetState(s.Key, "dismissed"); err != nil {
		t.Fatal(err)
	}
	_ = h.Record("a3", "folder2", moved("x@y.example", "gamma"))
	again, err := h.SuggestionForRecentMove("a3", "account1", "folder2")
	if err != nil || again != nil {
		t.Fatalf("dismissed suggestion came back: %v %v", again, err)
	}
}

func TestMoveHistoryIgnoresSameFolderNoopAndUnrelatedMail(t *testing.T) {
	h := NewMoveHistory(setupTestDB(t))
	noop := moved("a@b.example", "hello")
	noop[0].FolderID = "folder2"
	_ = h.Record("a1", "folder2", noop)
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM mail_filter_move_history`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no-op move recorded: %d %v", n, err)
	}
	_ = h.Record("a2", "folder2", moved("a@b.example", "Quarterly report"))
	_ = h.Record("a3", "folder2", moved("c@d.example", "Lunch menu"))
	s, _ := h.SuggestionForRecentMove("a3", "account1", "folder2")
	if s != nil {
		t.Fatalf("unrelated moves must not suggest: %+v", s)
	}
}
