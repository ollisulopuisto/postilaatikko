package appstate

import (
	"database/sql"
	"errors"
	"fmt"
)

const deviceStateImportVersion = 1

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

	var applied int
	err = tx.QueryRow("SELECT version FROM device_database_migrations WHERE version = ?", deviceStateImportVersion).Scan(&applied)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("finish device state initialization: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check device state import marker: %w", err)
	}

	if legacyDB != nil {
		var table string
		err = legacyDB.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'app_state'`).Scan(&table)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect legacy app-state table: %w", err)
		}
		if err == nil {
			rows, queryErr := legacyDB.Query("SELECT key, value FROM app_state ORDER BY key")
			if queryErr != nil {
				return fmt.Errorf("read legacy app-state rows: %w", queryErr)
			}
			for rows.Next() {
				var key, value string
				if scanErr := rows.Scan(&key, &value); scanErr != nil {
					rows.Close()
					return fmt.Errorf("scan legacy app-state row: %w", scanErr)
				}
				if _, insertErr := tx.Exec(`
					INSERT INTO app_state (key, value) VALUES (?, ?)
					ON CONFLICT(key) DO NOTHING
				`, key, value); insertErr != nil {
					rows.Close()
					return fmt.Errorf("import legacy app-state row %q: %w", key, insertErr)
				}
			}
			if rowsErr := rows.Err(); rowsErr != nil {
				rows.Close()
				return fmt.Errorf("read legacy app-state rows: %w", rowsErr)
			}
			if closeErr := rows.Close(); closeErr != nil {
				return fmt.Errorf("close legacy app-state rows: %w", closeErr)
			}
		}
	}

	if _, err := tx.Exec("INSERT INTO device_database_migrations (version) VALUES (?)", deviceStateImportVersion); err != nil {
		return fmt.Errorf("record device state import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit device state initialization: %w", err)
	}
	return nil
}
