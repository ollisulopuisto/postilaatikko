package settings

import "testing"

func TestQuickRepliesSaveListDelete(t *testing.T) {
	store := NewStore(openTestDB(t))

	got, err := store.ListQuickReplies()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty list: got %#v, want empty non-nil slice", got)
	}

	created, err := store.SaveQuickReply(QuickReply{Name: "Thanks", Body: "Thank you!\nBest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create: expected generated ID")
	}

	updated, err := store.SaveQuickReply(QuickReply{ID: created.ID, Name: "Thanks!", Body: "Many thanks"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("update changed ID: %q -> %q", created.ID, updated.ID)
	}

	got, err = store.ListQuickReplies()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Thanks!" || got[0].Body != "Many thanks" {
		t.Fatalf("after update: got %#v", got)
	}

	if err := store.DeleteQuickReply(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.DeleteQuickReply("missing"); err != nil {
		t.Fatalf("delete missing should be a no-op: %v", err)
	}
	got, _ = store.ListQuickReplies()
	if len(got) != 0 {
		t.Fatalf("after delete: got %#v", got)
	}
}

func TestSaveQuickReplyValidation(t *testing.T) {
	tests := []struct {
		name string
		in   QuickReply
	}{
		{name: "empty_name", in: QuickReply{Name: "  ", Body: "x"}},
		{name: "empty_body", in: QuickReply{Name: "x", Body: " \n "}},
		{name: "unknown_id", in: QuickReply{ID: "nope", Name: "x", Body: "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(openTestDB(t))
			if _, err := store.SaveQuickReply(tt.in); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestListQuickRepliesIgnoresCorruptValue(t *testing.T) {
	store := NewStore(openTestDB(t))
	if err := store.Set(KeyQuickReplies, "{not json"); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListQuickReplies()
	if err != nil {
		t.Fatalf("corrupt value should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
	}
}
