package database

import (
	"testing"

	"github.com/hkdb/aerion/internal/drafthistory"
)

// The PR #45 blob code paths must work on a database produced by the real
// migration chain (regression: migration 58 initially never landed, so these
// tables were missing on migrated databases).
func TestDraftAttachmentBlobPathsOnMigratedSchema(t *testing.T) {
	db := openUnmigratedTestDB(t)
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	store := drafthistory.NewStore(db.DB)
	if err := store.Clear("acc:d1"); err != nil {
		t.Fatalf("Clear (send/discard cleanup path): %v", err)
	}
	if _, err := store.PutBlob("a.pdf", "application/pdf", []byte{1, 2, 3}); err != nil {
		t.Fatalf("PutBlob (attachment snapshot path): %v", err)
	}
}
