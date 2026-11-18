package smtp

import (
	"encoding/json"
	"testing"
)

// ReplyMode carries the user's explicit reply / reply-all / forward choice
// through drafts and the pending-send payload, so the undo bar can name it.
func TestComposeMessageReplyModeSurvivesJSON(t *testing.T) {
	b, err := json.Marshal(ComposeMessage{ReplyMode: "reply-all"})
	if err != nil {
		t.Fatal(err)
	}
	var back ComposeMessage
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.ReplyMode != "reply-all" {
		t.Fatalf("ReplyMode = %q, want reply-all", back.ReplyMode)
	}
}
