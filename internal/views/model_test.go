package views

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// decodeStrict decodes a fixture into v, rejecting fields v does not define,
// and returns the fixture's raw JSON as a generic value for round-trip checks.
func decodeStrict(t *testing.T, name string, v any) any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("decode %s into %T: %v", name, v, err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode %s as generic JSON: %v", name, err)
	}
	return raw
}

func roundTrip(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("unmarshal re-encoded %T: %v", v, err)
	}
	return generic
}

// TestFixturesRoundTrip checks that the Go types describe every field in the
// shared fixtures, and that re-encoding reproduces them exactly (so optional
// fields stay omitted and slices stay arrays).
func TestFixturesRoundTrip(t *testing.T) {
	tests := []struct {
		fixture string
		target  func() any
	}{
		{"thread_duplicate_copies.json", func() any { return &ThreadView{} }},
		{"thread_partial_bodies.json", func() any { return &ThreadView{} }},
		{"page_all_inboxes_partial.json", func() any { return &ThreadPage{} }},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			target := tt.target()
			raw := decodeStrict(t, tt.fixture, target)
			if got := roundTrip(t, target); !reflect.DeepEqual(got, raw) {
				t.Fatalf("round trip differs\n got: %v\nwant: %v", got, raw)
			}
		})
	}
}

func loadThread(t *testing.T, name string) ThreadView {
	t.Helper()
	var view ThreadView
	decodeStrict(t, name, &view)
	return view
}

// TestThreadFixturesAreConsistent checks the derived fields the contract promises.
func TestThreadFixturesAreConsistent(t *testing.T) {
	for _, name := range []string{"thread_duplicate_copies.json", "thread_partial_bodies.json"} {
		t.Run(name, func(t *testing.T) {
			view := loadThread(t, name)
			if view.MessageCount != len(view.Messages) {
				t.Errorf("MessageCount = %d, want %d", view.MessageCount, len(view.Messages))
			}
			if got := len(MatchedRefs(view.Messages)); view.MatchedCount != got {
				t.Errorf("MatchedCount = %d, want %d", view.MatchedCount, got)
			}
			if got := CompletenessOf(view.Messages); !reflect.DeepEqual(got, view.Completeness) {
				t.Errorf("Completeness = %+v, want %+v", got, view.Completeness)
			}
			if !sort.SliceIsSorted(view.Messages, func(i, j int) bool {
				return view.Messages[i].Date.Before(view.Messages[j].Date)
			}) {
				t.Errorf("messages are not in chronological order")
			}
			last := view.Messages[len(view.Messages)-1].Date
			if !view.LatestDate.Equal(last) {
				t.Errorf("LatestDate = %v, want %v", view.LatestDate, last)
			}
		})
	}
}

// TestCopiesWithTheSameRFCMessageIDStayDistinct covers a Gmail-style thread
// where one email has two copies (Inbox and All Mail). They share an RFC
// Message-ID but are separate local messages, and only the in-scope copy is matched.
func TestCopiesWithTheSameRFCMessageIDStayDistinct(t *testing.T) {
	view := loadThread(t, "thread_duplicate_copies.json")
	if view.Messages[0].RFCMessageID != view.Messages[1].RFCMessageID {
		t.Fatal("fixture should have two copies sharing an RFC Message-ID")
	}
	keys := map[string]bool{}
	for _, m := range view.Messages {
		keys[m.Ref.Key()] = true
	}
	if len(keys) != len(view.Messages) {
		t.Fatalf("message refs collide: %d unique keys for %d messages", len(keys), len(view.Messages))
	}
	matched := MatchedRefs(view.Messages)
	if len(matched) != 1 || matched[0] != view.Messages[0].Ref {
		t.Fatalf("matched = %v, want only the Inbox copy %v", matched, view.Messages[0].Ref)
	}
}

// TestCrossAccountThreadsAreNotMerged covers the same email delivered to two
// accounts: both threads stay separate, account-qualified entries.
func TestCrossAccountThreadsAreNotMerged(t *testing.T) {
	var page ThreadPage
	decodeStrict(t, "page_all_inboxes_partial.json", &page)
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(page.Items))
	}
	if page.Items[0].Ref.Key() == page.Items[1].Ref.Key() {
		t.Fatal("thread refs from two accounts must not collide")
	}
	for _, item := range page.Items {
		for _, m := range item.Matched {
			if m.AccountID != item.Ref.AccountID {
				t.Errorf("matched message %v is outside its thread's account %s", m, item.Ref.AccountID)
			}
		}
	}
	if page.Complete {
		t.Error("a page with an unreachable account must not be complete")
	}
	if len(page.Errors) != 1 || page.Errors[0].AccountID != "acc-c" {
		t.Errorf("errors = %+v, want one error for acc-c", page.Errors)
	}
}

func TestCompletenessOf(t *testing.T) {
	cached := ThreadMessage{ContentState: ContentCached}
	missing := ThreadMessage{ContentState: ContentNotFetched}
	failed := ThreadMessage{ContentState: ContentFetchFailed}

	tests := []struct {
		name     string
		messages []ThreadMessage
		want     Completeness
	}{
		{"no messages is complete", nil, Completeness{Complete: true}},
		{"all cached", []ThreadMessage{cached, cached}, Completeness{Complete: true, Bodies: BodyCounts{Total: 2, Cached: 2}}},
		{"one not fetched", []ThreadMessage{cached, missing}, Completeness{Bodies: BodyCounts{Total: 2, Cached: 1, NotFetched: 1}}},
		{"one failed", []ThreadMessage{failed, cached}, Completeness{Bodies: BodyCounts{Total: 2, Cached: 1, FetchFailed: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompletenessOf(tt.messages); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestScopeValidate(t *testing.T) {
	inboxes := &Scope{Kind: ScopeAllInboxes}
	tests := []struct {
		name    string
		scope   Scope
		wantErr bool
	}{
		{"all inboxes", Scope{Kind: ScopeAllInboxes}, false},
		{"folder", Scope{Kind: ScopeFolder, AccountID: "acc-a", FolderID: "fld"}, false},
		{"folder without account", Scope{Kind: ScopeFolder, FolderID: "fld"}, true},
		{"folder without folder", Scope{Kind: ScopeFolder, AccountID: "acc-a"}, true},
		{"virtual folder", Scope{Kind: ScopeVirtualFolder, VirtualFolderID: "vf-1"}, false},
		{"virtual folder without id", Scope{Kind: ScopeVirtualFolder}, true},
		{"search with default scope", Scope{Kind: ScopeSearch, Query: "from:alice"}, false},
		{"search within all inboxes", Scope{Kind: ScopeSearch, Query: "x", Within: inboxes}, false},
		{"search without query", Scope{Kind: ScopeSearch, Query: "  "}, true},
		{"search within search", Scope{Kind: ScopeSearch, Query: "x", Within: &Scope{Kind: ScopeSearch, Query: "y"}}, true},
		{"search within invalid scope", Scope{Kind: ScopeSearch, Query: "x", Within: &Scope{Kind: ScopeFolder}}, true},
		{"unknown kind", Scope{Kind: "everything"}, true},
		{"empty kind", Scope{}, true},
		{"all inboxes with stray account", Scope{Kind: ScopeAllInboxes, AccountID: "acc-a"}, true},
		{"folder with stray query", Scope{Kind: ScopeFolder, AccountID: "a", FolderID: "f", Query: "x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scope.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestScopeFixtures checks the shared valid and invalid scope lists, which the
// TypeScript tests also check, so both languages accept and reject the same scopes.
func TestScopeFixtures(t *testing.T) {
	var fixture struct {
		Valid   []Scope `json:"valid"`
		Invalid []Scope `json:"invalid"`
	}
	decodeStrict(t, "scopes.json", &fixture)
	for i, s := range fixture.Valid {
		if err := s.Validate(); err != nil {
			t.Errorf("valid[%d] %+v rejected: %v", i, s, err)
		}
	}
	for i, s := range fixture.Invalid {
		if err := s.Validate(); err == nil {
			t.Errorf("invalid[%d] %+v accepted", i, s)
		}
	}
}

func TestScopeKey(t *testing.T) {
	inboxes := Scope{Kind: ScopeAllInboxes}
	explicit := Scope{Kind: ScopeSearch, Query: "from:alice", Within: &inboxes}
	implicit := Scope{Kind: ScopeSearch, Query: "from:alice"}

	if ScopeKey(explicit) != ScopeKey(implicit) {
		t.Error("a search with no scope must equal a search within all inboxes")
	}
	distinct := []Scope{
		inboxes,
		{Kind: ScopeFolder, AccountID: "acc-a", FolderID: "fld"},
		{Kind: ScopeFolder, AccountID: "acc-b", FolderID: "fld"},
		{Kind: ScopeVirtualFolder, VirtualFolderID: "fld"},
		implicit,
		{Kind: ScopeSearch, Query: "from:bob"},
		{Kind: ScopeSearch, Query: "from:alice", Within: &Scope{Kind: ScopeFolder, AccountID: "acc-a", FolderID: "fld"}},
	}
	seen := map[string]int{}
	for i, s := range distinct {
		key := ScopeKey(s)
		if j, dup := seen[key]; dup {
			t.Errorf("scopes %d and %d share the key %q", j, i, key)
		}
		seen[key] = i
		if ScopeKey(s) != key {
			t.Errorf("scope %d has an unstable key", i)
		}
	}
}

func TestCursor(t *testing.T) {
	scope := Scope{Kind: ScopeFolder, AccountID: "acc-a", FolderID: "fld"}
	other := Scope{Kind: ScopeFolder, AccountID: "acc-b", FolderID: "fld"}
	pos := Position{
		LatestDate: time.Date(2026, 10, 20, 9, 5, 0, 0, time.UTC),
		Thread:     ThreadRef{AccountID: "acc-a", ThreadID: "thr-100"},
	}

	cursor, err := EncodeCursor(scope, pos)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}

	t.Run("round trip", func(t *testing.T) {
		got, err := DecodeCursor(scope, cursor)
		if err != nil {
			t.Fatalf("DecodeCursor: %v", err)
		}
		if !got.LatestDate.Equal(pos.LatestDate) || got.Thread != pos.Thread {
			t.Fatalf("got %+v, want %+v", got, pos)
		}
	})

	t.Run("a cursor cannot be reused in another scope", func(t *testing.T) {
		if _, err := DecodeCursor(other, cursor); !errors.Is(err, ErrCursorScope) {
			t.Fatalf("err = %v, want ErrCursorScope", err)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		unsupported := base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"s":"x"}`))
		for name, bad := range map[string]string{
			"empty":       "",
			"not base64":  "!!!",
			"not json":    base64.RawURLEncoding.EncodeToString([]byte("hello")),
			"new version": unsupported,
		} {
			if _, err := DecodeCursor(scope, bad); !errors.Is(err, ErrCursorInvalid) {
				t.Errorf("%s: err = %v, want ErrCursorInvalid", name, err)
			}
		}
	})

	t.Run("an invalid scope cannot make a cursor", func(t *testing.T) {
		if _, err := EncodeCursor(Scope{Kind: ScopeFolder}, pos); err == nil {
			t.Fatal("expected an error for an invalid scope")
		}
	})
}

func TestRefKeysAreAccountQualified(t *testing.T) {
	a := MessageRef{AccountID: "acc-a", MessageID: "m-1"}
	b := MessageRef{AccountID: "acc-b", MessageID: "m-1"}
	if a.Key() == b.Key() {
		t.Error("message refs from two accounts with the same id must have different keys")
	}
	ta := ThreadRef{AccountID: "acc-a", ThreadID: "t"}
	tb := ThreadRef{AccountID: "acc-b", ThreadID: "t"}
	if ta.Key() == tb.Key() {
		t.Error("thread refs from two accounts with the same id must have different keys")
	}

	// A separator inside an id must not make two different refs share a key.
	left := MessageRef{AccountID: "a/b", MessageID: "c"}
	right := MessageRef{AccountID: "a", MessageID: "b/c"}
	if left.Key() == right.Key() {
		t.Error("message ref keys collide when ids contain the separator")
	}
}
