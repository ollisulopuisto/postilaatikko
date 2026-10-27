package message

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/email/reactions"
)

func TestReactionMetadataRoundTripAndLegacyNotification(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('account','Test','me@example.com','imap.example.com','smtp.example.com','me')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('folder','account','Inbox','INBOX','inbox')`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	for index, id := range []string{"structured", "legacy", "invalid"} {
		m := &Message{ID: id, AccountID: "account", FolderID: "folder", UID: uint32(index + 1), Date: time.Now(), FromName: "Example Sender", FromEmail: "sender@example.com", InReplyTo: "<target@example.com>", Subject: "Reaction"}
		if err = store.Create(m); err != nil {
			t.Fatal(err)
		}
	}
	expected := &reactions.Info{Emoji: "👍", TargetMessageID: "target@example.com", Provider: "gmail"}
	invalid := &reactions.Info{Invalid: true, Provider: "gmail"}
	if err = store.UpdateBodiesBatch([]BodyUpdate{{MessageID: "structured", BodyText: "Fallback", Reaction: expected}, {MessageID: "legacy", BodyText: "👍 Example Sender reacted to your message:"}, {MessageID: "invalid", BodyText: "👍 Example Sender reacted to your message:", Reaction: invalid}}); err != nil {
		t.Fatal(err)
	}
	var messages []*Message
	for _, id := range []string{"structured", "legacy", "invalid"} {
		m, err := store.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, m)
	}
	if err = store.EnrichReactions(messages); err != nil {
		t.Fatal(err)
	}
	if messages[0].Reaction == nil || *messages[0].Reaction != *expected {
		t.Fatalf("roundtrip: %+v", messages[0].Reaction)
	}
	if messages[1].Reaction == nil || messages[1].Reaction.Provider != "notification" {
		t.Fatal("legacy notification missed")
	}
	if messages[2].Reaction == nil || !messages[2].Reaction.Invalid {
		t.Fatal("invalid MIME fell back to notification")
	}
	if err = store.Delete("structured"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM message_reactions WHERE message_id='structured'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade: %d, %v", count, err)
	}
}
