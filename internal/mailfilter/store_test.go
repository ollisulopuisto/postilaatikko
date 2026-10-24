package mailfilter

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Accounts and folders to satisfy foreign-key references used by tests.
	_, err = db.Exec(`
		INSERT INTO accounts (id, name, email, imap_host, smtp_host, username)
		VALUES
			('account1', 'Account 1', 'user1@example.com', 'imap.example.com', 'smtp.example.com', 'user1'),
			('account2', 'Account 2', 'user2@example.com', 'imap.example.com', 'smtp.example.com', 'user2')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test accounts: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO folders (id, account_id, name, path, folder_type)
		VALUES
			('folder1', 'account1', 'Inbox', 'INBOX', 'inbox'),
			('folder2', 'account1', 'Work', 'Work', 'folder'),
			('folder3', 'account2', 'Inbox', 'INBOX', 'inbox')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test folders: %v", err)
	}

	return db
}

func TestStore_CreateGetList(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewStore(db)

	rules := []*Rule{
		{
			Name:            "Route invoices",
			Enabled:         true,
			OrderIndex:      0,
			AccountID:       "account1",
			FolderIDs:       []string{"folder1"},
			FromContains:    []string{"billing@example.com"},
			SubjectContains: []string{"invoice"},
			DestFolderID:    "folder3",
		},
		{
			Name:         "Route mailing lists",
			Enabled:      false,
			OrderIndex:   1,
			FolderIDs:    []string{"folder1", "folder2"},
			FromContains: []string{"lists@example.com"},
			DestFolderID: "folder3",
		},
	}

	for i, rule := range rules {
		if err := store.Create(rule); err != nil {
			t.Fatalf("Failed to create rule %d: %v", i, err)
		}
		if rule.ID == "" {
			t.Fatalf("Rule %d missing ID after Create", i)
		}
		if rule.CreatedAt.IsZero() || rule.UpdatedAt.IsZero() {
			t.Fatalf("Rule %d missing timestamps after Create", i)
		}
	}

	got, err := store.Get(rules[0].ID)
	if err != nil {
		t.Fatalf("Failed to get rule: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil for an existing rule")
	}
	if got.Name != "Route invoices" || got.AccountID != "account1" || got.DestFolderID != "folder3" {
		t.Fatalf("Unexpected rule after Get: %+v", got)
	}
	if !got.Enabled {
		t.Fatal("Enabled flag lost in round trip")
	}
	if len(got.FolderIDs) != 1 || got.FolderIDs[0] != "folder1" {
		t.Fatalf("FolderIDs lost in round trip: %v", got.FolderIDs)
	}
	if len(got.FromContains) != 1 || got.FromContains[0] != "billing@example.com" {
		t.Fatalf("FromContains lost in round trip: %v", got.FromContains)
	}
	if len(got.SubjectContains) != 1 || got.SubjectContains[0] != "invoice" {
		t.Fatalf("SubjectContains lost in round trip: %v", got.SubjectContains)
	}

	missing, err := store.Get("does-not-exist")
	if err != nil {
		t.Fatalf("Get on missing rule returned error: %v", err)
	}
	if missing != nil {
		t.Fatalf("Expected nil for missing rule, got %+v", missing)
	}
}

func TestStore_ListAndListEnabled(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewStore(db)

	first := &Rule{Name: "First", Enabled: true, OrderIndex: 0, DestFolderID: "folder3"}
	second := &Rule{Name: "Second", Enabled: false, OrderIndex: 1, DestFolderID: "folder3"}
	if err := store.Create(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(second); err != nil {
		t.Fatal(err)
	}

	all, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 2 || all[0].ID != first.ID || all[1].ID != second.ID {
		t.Fatalf("List ordering broken: %+v", all)
	}

	enabled, err := store.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled failed: %v", err)
	}
	if len(enabled) != 1 || enabled[0].ID != first.ID {
		t.Fatalf("ListEnabled should return only the enabled rule: %+v", enabled)
	}
}

func TestStore_UpdateAndDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewStore(db)

	rule := &Rule{Name: "Before", Enabled: true, OrderIndex: 0, FromContains: []string{"a@x"}, DestFolderID: "folder3"}
	if err := store.Create(rule); err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Hour)
	rule.Name = "After"
	rule.Enabled = false
	rule.FromContains = []string{"b@x", "c@x"}
	rule.UpdatedAt = before // must be overwritten by Update
	if err := store.Update(rule); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := store.Get(rule.ID)
	if err != nil || got == nil {
		t.Fatalf("Get after Update failed: %v", err)
	}
	if got.Name != "After" || got.Enabled {
		t.Fatalf("Update did not persist: %+v", got)
	}
	if len(got.FromContains) != 2 || got.FromContains[0] != "b@x" {
		t.Fatalf("FromContains not replaced: %v", got.FromContains)
	}
	if got.UpdatedAt.Before(before) {
		t.Fatal("Update did not refresh UpdatedAt")
	}

	if err := store.Update(&Rule{ID: "does-not-exist", Name: "Ghost"}); err == nil {
		t.Fatal("Update on missing rule should fail")
	}

	if err := store.Delete(rule.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	gone, err := store.Get(rule.ID)
	if err != nil || gone != nil {
		t.Fatalf("Rule still present after Delete: %v %+v", err, gone)
	}
}

func TestStore_Reorder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewStore(db)

	first := &Rule{Name: "First", Enabled: true, OrderIndex: 0, DestFolderID: "folder3"}
	second := &Rule{Name: "Second", Enabled: true, OrderIndex: 1, DestFolderID: "folder3"}
	if err := store.Create(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(second); err != nil {
		t.Fatal(err)
	}

	if err := store.Reorder([]string{second.ID, first.ID}); err != nil {
		t.Fatalf("Reorder failed: %v", err)
	}
	all, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("Reorder did not change list order: %+v", all)
	}

	if err := store.Reorder([]string{"does-not-exist"}); err == nil {
		t.Fatal("Reorder with unknown ID should fail")
	}
}

func TestStore_CreateRejectsEmptyName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewStore(db)

	if err := store.Create(&Rule{Name: "   ", DestFolderID: "folder3"}); err == nil {
		t.Fatal("Create with blank name should fail")
	}
}
