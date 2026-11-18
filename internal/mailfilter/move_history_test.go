package mailfilter

import (
	"encoding/json"
	"testing"

	"github.com/hkdb/aerion/internal/message"
)

func moved(from, subject string) []*message.Message {
	return []*message.Message{{
		AccountID: "account1", FolderID: "folder1", FromEmail: from, Subject: subject,
		ToList: `[{"name":"","email":"user1@example.com"}]`, CcList: "[]",
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

func withRecipients(from, subject, to, cc string) []*message.Message {
	m := moved(from, subject)
	m[0].ToList, m[0].CcList = to, cc
	return m
}

func recipientSuggestion(t *testing.T, prior, current []*message.Message) *SuggestedFilter {
	t.Helper()
	h := NewMoveHistory(setupTestDB(t))
	if err := h.Record("a1", "folder2", prior); err != nil {
		t.Fatal(err)
	}
	if err := h.Record("a2", "folder2", current); err != nil {
		t.Fatal(err)
	}
	s, err := h.SuggestionForRecentMove("a2", "account1", "folder2")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMoveHistoryRecipientOnlySuggestions(t *testing.T) {
	list := `[{"name":"","email":"team@lists.example"}]`
	other := `[{"name":"","email":"someone@else.example"}]`
	cases := []struct {
		name     string
		pTo, pCc string
		cTo, cCc string
		want     string
	}{
		{"to and to", list, "[]", list, "[]", "team@lists.example"},
		{"cc and cc", "[]", list, "[]", list, "team@lists.example"},
		{"to and cc", list, "[]", "[]", list, "team@lists.example"},
		{"nothing shared", list, "[]", other, "[]", ""},
	}
	for _, c := range cases {
		s := recipientSuggestion(t,
			withRecipients("a@one.example", "alpha", c.pTo, c.pCc),
			withRecipients("b@two.example", "omega", c.cTo, c.cCc))
		if c.want == "" {
			if s != nil {
				t.Fatalf("%s: unexpected suggestion %+v", c.name, s)
			}
			continue
		}
		if s == nil || len(s.ToContains) != 1 || s.ToContains[0] != c.want || len(s.FromContains) != 0 {
			t.Fatalf("%s: got %+v", c.name, s)
		}
	}
}

func TestMoveHistoryOwnAddressIsNotARecipientSignal(t *testing.T) {
	me := `[{"name":"","email":"User1@example.com"}]`
	s := recipientSuggestion(t,
		withRecipients("a@one.example", "alpha", me, "[]"),
		withRecipients("b@two.example", "omega", me, "[]"))
	if s != nil {
		t.Fatalf("own address produced a suggestion: %+v", s)
	}
}

func TestMoveHistoryComparesEverySampleOfABulkMove(t *testing.T) {
	prior := moved("news@shop.example", "alpha")
	current := append(moved("x@unrelated.example", "zzzz"), moved("news@shop.example", "beta")...)
	s := recipientSuggestion(t, prior, current)
	if s == nil || len(s.FromContains) != 1 || s.FromContains[0] != "news@shop.example" {
		t.Fatalf("matching non-first sample was ignored: %+v", s)
	}
}

func TestSuggestionJSONCarriesArraysNotNull(t *testing.T) {
	s := recipientSuggestion(t,
		moved("news@shop.example", "alpha"), moved("news@shop.example", "beta"))
	if s == nil {
		t.Fatal("want suggestion")
	}
	raw, _ := json.Marshal(s)
	for _, k := range []string{"toContains", "subjectContains"} {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(raw, &m)
		if string(m[k]) == "null" {
			t.Fatalf("%s serialised as null: %s", k, raw)
		}
	}
}
