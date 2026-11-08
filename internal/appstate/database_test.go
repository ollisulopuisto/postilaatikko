package appstate

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/database"
)

func TestInitializeDeviceDatabaseCopiesSettingsAndImageAllowlistOnce(t *testing.T) {
	legacy, err := database.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	for _, statement := range []string{
		`CREATE TABLE app_state (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE image_allowlist (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL, value TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE folder_content_policies (folder_id TEXT PRIMARY KEY, account_id TEXT NOT NULL, body_prefetch INTEGER NOT NULL, updated_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`INSERT INTO app_state(key,value) VALUES ('layout','compact')`,
		`INSERT INTO settings(key,value) VALUES ('theme_mode','light'), ('read_receipt_response_policy','ask')`,
		`INSERT INTO image_allowlist(id,type,value,created_at) VALUES (9,'domain','trusted.example','2026-11-01 12:00:00')`,
		`INSERT INTO folder_content_policies(folder_id,account_id,body_prefetch,updated_at) VALUES ('folder-1','account-1',0,'2026-11-02 10:00:00')`,
	} {
		if _, err := legacy.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	device, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if _, err := device.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`INSERT INTO settings(key,value) VALUES ('theme_mode','dark')`); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeviceDatabase(device.DB, legacy.DB); err != nil {
		t.Fatal(err)
	}
	assertStoredValue(t, device.DB, `SELECT value FROM app_state WHERE key='layout'`, "compact")
	assertStoredValue(t, device.DB, `SELECT value FROM settings WHERE key='theme_mode'`, "dark")
	assertStoredValue(t, device.DB, `SELECT value FROM settings WHERE key='read_receipt_response_policy'`, "ask")
	assertStoredValue(t, device.DB, `SELECT value FROM image_allowlist WHERE id=9`, "trusted.example")
	var bodyPrefetch bool
	if err := device.QueryRow(`SELECT body_prefetch FROM folder_content_policies WHERE folder_id='folder-1'`).Scan(&bodyPrefetch); err != nil {
		t.Fatal(err)
	}
	if bodyPrefetch {
		t.Fatal("disabled body-prefetch override imported as enabled")
	}
	assertStoredValue(t, legacy.DB, `SELECT value FROM settings WHERE key='theme_mode'`, "light")
	assertStoredValue(t, legacy.DB, `SELECT value FROM image_allowlist WHERE id=9`, "trusted.example")

	if _, err := device.Exec(`DELETE FROM settings WHERE key='read_receipt_response_policy'`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`DELETE FROM image_allowlist WHERE id=9`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`DELETE FROM folder_content_policies WHERE folder_id='folder-1'`); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeviceDatabase(device.DB, legacy.DB); err != nil {
		t.Fatal(err)
	}
	assertNoRows(t, device.DB, `SELECT value FROM settings WHERE key='read_receipt_response_policy'`)
	assertNoRows(t, device.DB, `SELECT value FROM image_allowlist WHERE id=9`)
	assertNoRows(t, device.DB, `SELECT body_prefetch FROM folder_content_policies WHERE folder_id='folder-1'`)
}

func TestInitializeDeviceDatabaseUpgradesSettingsAfterAppStateImport(t *testing.T) {
	legacy, err := database.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO settings(key,value) VALUES ('language','fi')`); err != nil {
		t.Fatal(err)
	}

	device, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if _, err := device.Exec(`CREATE TABLE device_database_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`INSERT INTO device_database_migrations(version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeviceDatabase(device.DB, legacy.DB); err != nil {
		t.Fatal(err)
	}
	assertStoredValue(t, device.DB, `SELECT value FROM settings WHERE key='language'`, "fi")
}

func TestDeviceOnlyInitializationLeavesLegacyImportsPending(t *testing.T) {
	device, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()

	if err := InitializeDeviceDatabase(device.DB, nil); err != nil {
		t.Fatal(err)
	}
	var markerCount int
	if err := device.QueryRow(`SELECT count(*) FROM device_database_migrations`).Scan(&markerCount); err != nil {
		t.Fatal(err)
	}
	if markerCount != 0 {
		t.Fatalf("device-only initialization recorded %d legacy import markers", markerCount)
	}

	legacy, err := database.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO settings(key, value) VALUES ('language', 'fi')`); err != nil {
		t.Fatal(err)
	}
	if err := ImportLegacyDeviceDatabase(device.DB, legacy.DB); err != nil {
		t.Fatal(err)
	}
	assertStoredValue(t, device.DB, `SELECT value FROM settings WHERE key='language'`, "fi")
	assertStoredValue(t, legacy.DB, `SELECT value FROM settings WHERE key='language'`, "fi")
	if err := device.QueryRow(`SELECT count(*) FROM device_database_migrations`).Scan(&markerCount); err != nil {
		t.Fatal(err)
	}
	if markerCount != 4 {
		t.Fatalf("legacy import recorded %d markers, want 4", markerCount)
	}
}

func TestInitializeDeviceDatabaseCopiesTrustedCertificatesOnce(t *testing.T) {
	legacy, err := database.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(`CREATE TABLE trusted_certificates (
		id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, host TEXT NOT NULL,
		subject TEXT NOT NULL, issuer TEXT NOT NULL, not_before DATETIME,
		not_after DATETIME, accepted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(host, fingerprint)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO trusted_certificates
		(id, fingerprint, host, subject, issuer, accepted_at)
		VALUES ('legacy-id', 'legacy-fingerprint', 'mail.example.com', 'legacy subject', 'legacy issuer', '2026-11-03 10:00:00')`); err != nil {
		t.Fatal(err)
	}

	device, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if _, err := device.Exec(`CREATE TABLE device_database_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`INSERT INTO device_database_migrations(version) VALUES (1), (2), (3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`CREATE TABLE trusted_certificates (
		id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, host TEXT NOT NULL,
		subject TEXT NOT NULL, issuer TEXT NOT NULL, not_before DATETIME,
		not_after DATETIME, accepted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(host, fingerprint)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Exec(`INSERT INTO trusted_certificates
		(id, fingerprint, host, subject, issuer)
		VALUES ('device-id', 'device-fingerprint', 'local.example.com', 'device subject', 'device issuer')`); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeviceDatabase(device.DB, legacy.DB); err != nil {
		t.Fatal(err)
	}
	assertStoredValue(t, device.DB, `SELECT subject FROM trusted_certificates WHERE host='mail.example.com'`, "legacy subject")
	assertStoredValue(t, device.DB, `SELECT subject FROM trusted_certificates WHERE host='local.example.com'`, "device subject")
	assertStoredValue(t, legacy.DB, `SELECT subject FROM trusted_certificates WHERE id='legacy-id'`, "legacy subject")

	if _, err := device.Exec(`DELETE FROM trusted_certificates WHERE id='legacy-id'`); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeviceDatabase(device.DB, legacy.DB); err != nil {
		t.Fatal(err)
	}
	assertNoRows(t, device.DB, `SELECT id FROM trusted_certificates WHERE id='legacy-id'`)
}

func assertStoredValue(t *testing.T, db *sql.DB, query, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("stored value = %q, want %q", got, want)
	}
}

func assertNoRows(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	var value string
	if err := db.QueryRow(query).Scan(&value); err == nil {
		t.Fatalf("unexpected stored value %q", value)
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("query failed unexpectedly: %v", err)
	}
}
