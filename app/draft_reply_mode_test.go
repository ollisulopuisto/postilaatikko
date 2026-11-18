package app

import (
	"testing"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/smtp"
)

// Remote and detached editors read drafts through the helper document, so a
// Reply All draft reduced to one recipient must keep its mode there.
func TestDraftFromHelperDocumentKeepsReplyMode(t *testing.T) {
	got := draftFromHelperDocument(&v1.DraftDocument{ID: "d1", AccountID: "a", Message: smtp.ComposeMessage{
		ReplyMode: "reply-all", To: []smtp.Address{{Address: "one@example.com"}},
	}})
	if got == nil || got.ReplyType != "reply-all" {
		t.Fatalf("ReplyType = %+v", got)
	}
}
