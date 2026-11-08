package appstate

import (
	"database/sql"
	"errors"
	"fmt"
)

const (
	deviceStateImportVersion    = 1
	deviceSettingsImportVersion = 2
)

// InitializeDeviceDatabase creates the device-owned app-state schema and
// imports legacy rows from the mailbox database once. Legacy rows are retained
// so an interrupted or rolled-back upgrade cannot lose the original state.
func InitializeDeviceDatabase(deviceDB, legacyDB *sql.DB) error {
	if deviceDB == nil {
		return fmt.Errorf("device database is required")
	}
	if _, err := deviceDB.Exec(`
		CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create device app-state table: %w", err)
	}
	if _, err := deviceDB.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create device settings table: %w", err)
	}
	if _, err := deviceDB.Exec(`
		CREATE TABLE IF NOT EXISTS image_allowlist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL CHECK(type IN ('domain', 'sender')),
			value TEXT NOT NULL COLLATE NOCASE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(type, value)
		)
	`); err != nil {
		return fmt.Errorf("create device image allowlist table: %w", err)
	}
	if _, err := deviceDB.Exec(`CREATE INDEX IF NOT EXISTS idx_device_image_allowlist_type_value ON image_allowlist(type, value)`); err != nil {
		return fmt.Errorf("create device image allowlist index: %w", err)
	}
	if _, err := deviceDB.Exec(`
		CREATE TABLE IF NOT EXISTS device_database_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create device database migration table: %w", err)
	}

	tx, err := deviceDB.Begin()
	if err != nil {
		return fmt.Errorf("begin device state initialization: %w", err)
	}
	defer tx.Rollback()

	if err := importAppStateOnce(tx, legacyDB); err != nil {
		return err
	}
	if err := importDeviceSettingsOnce(tx, legacyDB); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit device state initialization: %w", err)
	}
	return nil
}

func importAppStateOnce(tx *sql.Tx, legacyDB *sql.DB) error {
	applied, err := deviceMigrationApplied(tx, deviceStateImportVersion)
	if err != nil {
		return fmt.Errorf("check app-state import marker: %w", err)
	}
	if applied {
		return nil
	}
	if legacyDB != nil {
		rows, err := queryIfTableExists(legacyDB, "app_state", "SELECT key, value FROM app_state ORDER BY key")
		if err != nil {
			return fmt.Errorf("read legacy app-state rows: %w", err)
		}
		if rows != nil {
			for rows.Next() {
				var key, value string
				if err := rows.Scan(&key, &value); err != nil {
					rows.Close()
					return fmt.Errorf("scan legacy app-state row: %w", err)
				}
				if _, err := tx.Exec(`INSERT INTO app_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`, key, value); err != nil {
					rows.Close()
					return fmt.Errorf("import legacy app-state row %q: %w", key, err)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return fmt.Errorf("read legacy app-state rows: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("close legacy app-state rows: %w", err)
			}
		}
	}
	return recordDeviceMigration(tx, deviceStateImportVersion)
}

func importDeviceSettingsOnce(tx *sql.Tx, legacyDB *sql.DB) error {
	applied, err := deviceMigrationApplied(tx, deviceSettingsImportVersion)
	if err != nil {
		return fmt.Errorf("check settings import marker: %w", err)
	}
	if applied {
		return nil
	}
	if legacyDB != nil {
		settingsRows, err := queryIfTableExists(legacyDB, "settings", "SELECT key, value FROM settings ORDER BY key")
		if err != nil {
			return fmt.Errorf("read legacy settings rows: %w", err)
		}
		if settingsRows != nil {
			for settingsRows.Next() {
				var key, value string
				if err := settingsRows.Scan(&key, &value); err != nil {
					settingsRows.Close()
					return fmt.Errorf("scan legacy setting: %w", err)
				}
				if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`, key, value); err != nil {
					settingsRows.Close()
					return fmt.Errorf("import legacy setting %q: %w", key, err)
				}
			}
			if err := settingsRows.Err(); err != nil {
				settingsRows.Close()
				return fmt.Errorf("read legacy settings rows: %w", err)
			}
			if err := settingsRows.Close(); err != nil {
				return fmt.Errorf("close legacy settings rows: %w", err)
			}
		}

		allowlistRows, err := queryIfTableExists(legacyDB, "image_allowlist", "SELECT id, type, value, created_at FROM image_allowlist ORDER BY id")
		if err != nil {
			return fmt.Errorf("read legacy image allowlist rows: %w", err)
		}
		if allowlistRows != nil {
			for allowlistRows.Next() {
				var id int64
				var entryType, value string
				var createdAt sql.NullString
				if err := allowlistRows.Scan(&id, &entryType, &value, &createdAt); err != nil {
					allowlistRows.Close()
					return fmt.Errorf("scan legacy image allowlist row: %w", err)
				}
				var createdAtValue any
				if createdAt.Valid {
					createdAtValue = createdAt.String
				}
				if _, err := tx.Exec(`INSERT OR IGNORE INTO image_allowlist (id, type, value, created_at) VALUES (?, ?, ?, ?)`, id, entryType, value, createdAtValue); err != nil {
					allowlistRows.Close()
					return fmt.Errorf("import legacy image allowlist row %d: %w", id, err)
				}
			}
			if err := allowlistRows.Err(); err != nil {
				allowlistRows.Close()
				return fmt.Errorf("read legacy image allowlist rows: %w", err)
			}
			if err := allowlistRows.Close(); err != nil {
				return fmt.Errorf("close legacy image allowlist rows: %w", err)
			}
		}
	}
	return recordDeviceMigration(tx, deviceSettingsImportVersion)
}

func deviceMigrationApplied(tx *sql.Tx, version int) (bool, error) {
	var applied int
	err := tx.QueryRow("SELECT version FROM device_database_migrations WHERE version = ?", version).Scan(&applied)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func recordDeviceMigration(tx *sql.Tx, version int) error {
	if _, err := tx.Exec("INSERT INTO device_database_migrations (version) VALUES (?)", version); err != nil {
		return fmt.Errorf("record device database migration %d: %w", version, err)
	}
	return nil
}

func queryIfTableExists(db *sql.DB, tableName, query string) (*sql.Rows, error) {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return db.Query(query)
}
