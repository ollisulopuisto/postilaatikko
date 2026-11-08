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
