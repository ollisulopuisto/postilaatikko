package message

import "testing"

func TestDeduplicateThreadMessagesPreservesScopeAndAccountBoundaries(t *testing.T) {
	context := &Message{ID: "context", AccountID: "a", MessageID: "<same@example.org>", BodyFetched: true}
	matched := &Message{ID: "matched", AccountID: "a", MessageID: "same@example.org"}
	other := &Message{ID: "other", AccountID: "b", MessageID: "<same@example.org>"}
	missing := &Message{ID: "missing", AccountID: "a"}
	missing2 := &Message{ID: "missing2", AccountID: "a"}
	result := DeduplicateThreadMessages([]*Message{context, other, matched, missing, missing2}, "", map[string]bool{"matched": true})
	if len(result) != 4 || result[0] != matched || result[1] != other || result[2] != missing || result[3] != missing2 {
		t.Fatalf("scope, account or missing-RFC-ID identity lost: %+v", result)
	}
	if context.BodyFetched != true || matched.BodyFetched != false {
		t.Fatal("selection mutated source copies")
	}
}

func TestDeduplicateThreadMessagesPrefersLiveOverCachedSnapshot(t *testing.T) {
	var snapshotUID int32 = -7
	snapshot := &Message{ID: "snapshot", AccountID: "a", FolderID: "inbox", MessageID: "same@test", UID: uint32(snapshotUID), BodyFetched: true}
	live := &Message{ID: "live", AccountID: "a", FolderID: "inbox", MessageID: "same@test", UID: 7}
	for _, copies := range [][]*Message{{snapshot, live}, {live, snapshot}} {
		got := DeduplicateThreadMessages(copies, "inbox", nil)
		if len(got) != 1 || got[0] != live {
			t.Fatal("cached snapshot hid live reader target", got)
		}
	}
}
