//go:build linux

package app

import (
	"context"
	"github.com/hkdb/aerion/internal/appstate"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/settings"
	gokeyring "github.com/zalando/go-keyring"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Preflight now delegates migrations to the bundled helper. Test the real
// executable contract instead of failing early because go test built only app.
func installPreflightTestHelper(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(filepath.Dir(executable), "postilaatikko-helper")
	if _, err := os.Stat(helper); err == nil {
		return
	}
	command := exec.Command("go", "build", "-o", helper, "../cmd/postilaatikko-helper")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build preflight helper: %v\n%s", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(helper) })
}

func TestFailedPreflightReleasesDatabase(t *testing.T) {
	installPreflightTestHelper(t)
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	data := filepath.Join(dir, "data", "aerion")
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "device.key")
	if err := os.WriteFile(path, []byte("synthetic corrupt key"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	if err := a.Preflight(); err == nil || !strings.Contains(err.Error(), "device key") {
		t.Fatalf("expected device key validation failure, got %v", err)
	}
	if a.db != nil {
		t.Fatal("partially initialized database still published")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "synthetic corrupt key" {
		t.Fatal("key changed", err)
	}
}

func TestRemotePreflightDoesNotOpenMailboxDatabase(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, filepath.Join(dir, name))
	}
	data := filepath.Join(dir, "XDG_DATA_HOME", "aerion")
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	deviceDB, err := database.Open(filepath.Join(data, "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deviceDB.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := deviceDB.Exec(`INSERT INTO settings(key, value) VALUES (?, ?)`, settings.KeyMailboxServiceEndpoint, "https://mail.example.test:8029"); err != nil {
		t.Fatal(err)
	}
	if err := deviceDB.Close(); err != nil {
		t.Fatal(err)
	}

	a := &App{}
	if err := a.Preflight(); err != nil {
		t.Fatal(err)
	}
	defer a.deviceDB.Close()
	if !a.remoteServiceMode || a.remoteServiceEndpoint != "https://mail.example.test:8029" {
		t.Fatalf("remote mode = %t, endpoint = %q", a.remoteServiceMode, a.remoteServiceEndpoint)
	}
	if a.db != nil || a.credStore != nil || a.schemaOwner != nil {
		t.Fatal("remote preflight initialized local service database resources")
	}
	if _, err := os.Stat(a.paths.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("mailbox database exists after remote preflight: stat err=%v", err)
	}
}

func TestRemoteStartupSignalsReadyWithoutMailboxStores(t *testing.T) {
	deviceDB, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer deviceDB.Close()
	if err := appstate.InitializeDeviceDatabaseSchema(deviceDB.DB); err != nil {
		t.Fatal(err)
	}
	readyEvent := false
	a := &App{
		deviceDB:              deviceDB,
		remoteServiceMode:     true,
		remoteServiceEndpoint: "https://mail.example.test:8029",
		runtimeEventEmitter: func(name string, _ ...interface{}) {
			readyEvent = name == "app:ready"
		},
		mailboxService: &mailboxServiceConnector{
			client: automationclient.New(automationclient.Config{Endpoint: "https://mail.example.test:8029", Token: "device-token"}),
		},
	}
	a.startRemoteWorkspace(context.Background())
	if !a.IsReady() || !readyEvent {
		t.Fatalf("ready=%t readyEvent=%t", a.IsReady(), readyEvent)
	}
	if a.db != nil || a.accountStore != nil || a.folderStore != nil || a.messageStore != nil || a.syncEngine != nil || a.credStore != nil {
		t.Fatal("remote startup constructed local mailbox stores or workers")
	}
}

func TestPreflightIsIdempotent(t *testing.T) {
	installPreflightTestHelper(t)
	dir := t.TempDir()
	for _, name := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, filepath.Join(dir, name))
	}
	gokeyring.MockInit()
	a := &App{}
	if err := a.Preflight(); err != nil {
		t.Fatal(err)
	}
	defer a.db.Close()
	defer a.deviceDB.Close()
	defer func() {
		if err := a.schemaOwner.Release(); err != nil {
			t.Errorf("release schema owner: %v", err)
		}
	}()
	db, credentials := a.db, a.credStore
	if err := a.Preflight(); err != nil {
		t.Fatal(err)
	}
	if a.db != db || a.credStore != credentials {
		t.Fatal("repeated bridge call replaced initialized stores")
	}
}
