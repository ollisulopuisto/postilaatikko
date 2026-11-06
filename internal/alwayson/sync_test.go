package alwayson

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/imap"
)

func TestSyncRuntimeStartsAndStopsWithoutAccounts(t *testing.T) {
	dataDir := t.TempDir()
	db, err := database.Open(filepath.Join(dataDir, "aerion.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	runtime, err := OpenSyncRuntime(dataDir, func(context.Context, string) (*imap.ClientConfig, error) {
		t.Fatal("no credentials should be requested without configured accounts")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime.Start(ctx)
	cancel()
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenSyncRuntimeRequiresConfiguredDatabase(t *testing.T) {
	_, err := OpenSyncRuntime(t.TempDir(), func(context.Context, string) (*imap.ClientConfig, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected missing database to be rejected")
	}
}
