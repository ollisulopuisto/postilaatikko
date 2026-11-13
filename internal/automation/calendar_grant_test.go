package automation

import (
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"path/filepath"
	"testing"
)

func TestCalendarReadGrantPersists(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteTokenStore(db)
	grant, err := store.RegisterClient("calendar", []v1.Capability{v1.Capability("calendar.read")})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.GetGrant(grant.Token)
	if err != nil || saved == nil || len(saved.Capabilities) != 1 || saved.Capabilities[0] != v1.Capability("calendar.read") {
		t.Fatalf("grant=%v err=%v", saved, err)
	}
}
