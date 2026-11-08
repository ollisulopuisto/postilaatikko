package folder

import (
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/appstate"
	"github.com/hkdb/aerion/internal/database"
)

func TestContentPolicyStoreUsesDeviceDatabaseAndPurgesRemovedAccount(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := appstate.InitializeDeviceDatabase(db.DB, nil); err != nil {
		t.Fatal(err)
	}
	store := NewContentPolicyStore(db)
	if err := store.Set("account-a", "folder-a", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("account-b", "folder-b", true); err != nil {
		t.Fatal(err)
	}
	policy, err := store.Get("account-a", "folder-a")
	if err != nil || policy == nil || policy.Enabled {
		t.Fatalf("disabled override = %#v, err=%v", policy, err)
	}
	if err := store.DeleteAccount("account-a"); err != nil {
		t.Fatal(err)
	}
	policy, err = store.Get("account-a", "folder-a")
	if err != nil || policy != nil {
		t.Fatalf("removed account policy = %#v, err=%v; want absent", policy, err)
	}
	policy, err = store.Get("account-b", "folder-b")
	if err != nil || policy == nil || !policy.Enabled {
		t.Fatalf("other account policy changed: %#v, err=%v", policy, err)
	}
}
