package app

import (
	"testing"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
)

// Moves other than Inbox <-> Spam must never use the Gmail label path. These
// cases return before the account or token stores are consulted.
func TestGmailSpamMoveOnlyForInboxAndSpam(t *testing.T) {
	msgs := []*message.Message{{ID: "1", MessageID: "<a@x>"}}
	tests := []struct {
		name         string
		source, dest folder.Type
	}{
		{"inbox to trash", folder.TypeInbox, folder.TypeTrash},
		{"inbox to archive", folder.TypeInbox, folder.TypeArchive},
		{"inbox to user folder", folder.TypeInbox, folder.TypeFolder},
		{"user folder to spam", folder.TypeFolder, folder.TypeSpam},
		{"spam to trash", folder.TypeSpam, folder.TypeTrash},
		{"spam to user folder", folder.TypeSpam, folder.TypeFolder},
		{"sent to spam", folder.TypeSent, folder.TypeSpam},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &App{}
			ids, spam, ok := a.gmailSpamMove("acc", msgs, &folder.Folder{Type: tt.source}, &folder.Folder{Type: tt.dest})
			if ok || spam || ids != nil {
				t.Fatalf("gmailSpamMove = (%v, %v, %v), want it to decline", ids, spam, ok)
			}
		})
	}
}
