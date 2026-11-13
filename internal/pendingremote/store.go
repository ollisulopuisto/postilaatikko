// Package pendingremote stores a device-owned Undo Send safety window without
// opening a mailbox database. Only the helper performs provider delivery.
package pendingremote

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	MaxPreparedBytes = 32 << 20
	MaxPayloadBytes  = 20 << 20
	MaxStoredBytes   = 64 << 20
	MaxRows          = 10
	StateUnknown     = "unknown"
	StateSent        = "sent"
	StateFailed      = "failed"
)

type Item struct {
	ID         string
	AccountID  string
	DraftID    string
	Payload    string
	Prepared   []byte
	DispatchAt time.Time
	CreatedAt  time.Time
}
type Store struct{ db *sql.DB }

func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("device pending-send database is unavailable")
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS remote_pending_sends (
 id TEXT PRIMARY KEY, account_id TEXT NOT NULL, draft_id TEXT NOT NULL,
 payload TEXT NOT NULL, prepared BLOB NOT NULL, dispatch_at TEXT NOT NULL,
 created_at TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('pending','sending','unknown','sent','failed','cancelled','expired')));
 CREATE UNIQUE INDEX IF NOT EXISTS remote_pending_draft ON remote_pending_sends(account_id,draft_id)
 WHERE draft_id != '' AND state IN ('pending','sending','unknown');`)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Queue(item Item) error {
	if item.ID == "" || item.AccountID == "" || item.Payload == "" || len(item.Payload) > MaxPayloadBytes || len(item.Prepared) == 0 || len(item.Prepared) > MaxPreparedBytes || !item.DispatchAt.After(time.Now()) {
		return errors.New("pending send requires account, bounded payload and prepared bytes, and a future dispatch time")
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	// Single-statement quota check and insert serialize concurrent queue attempts.
	// Prune content-free terminal rows; unknown outcomes retain their draft lock.
	if _, err := s.db.Exec(`DELETE FROM remote_pending_sends WHERE state IN ('sent','failed','cancelled','expired')`); err != nil {
		return err
	}
	result, err := s.db.Exec(`INSERT INTO remote_pending_sends(id,account_id,draft_id,payload,prepared,dispatch_at,created_at,state)
 SELECT ?,?,?,?,?,?,?,'pending'
 WHERE (SELECT count(*) FROM remote_pending_sends WHERE state IN ('pending','sending','unknown')) < ?
 AND (SELECT COALESCE(sum(length(CAST(payload AS BLOB))+length(prepared)),0) FROM remote_pending_sends) + ? <= ?`,
		item.ID, item.AccountID, item.DraftID, item.Payload, item.Prepared, item.DispatchAt.UTC().Format(time.RFC3339Nano), item.CreatedAt.UTC().Format(time.RFC3339Nano), MaxRows, len(item.Payload)+len(item.Prepared), MaxStoredBytes)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("device pending-send queue is full; resolve pending or uncertain sends first")
	}
	return nil
}
func (s *Store) Claim(id string) (*Item, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(`UPDATE remote_pending_sends SET state='sending' WHERE id=? AND state='pending'`, id)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, errors.New("message is no longer pending; do not submit again")
	}
	item, err := scanItem(tx.QueryRow(`SELECT id,account_id,draft_id,payload,prepared,dispatch_at,created_at FROM remote_pending_sends WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}
func scanItem(row interface{ Scan(...any) error }) (*Item, error) {
	var item Item
	var due, created string
	if err := row.Scan(&item.ID, &item.AccountID, &item.DraftID, &item.Payload, &item.Prepared, &due, &created); err != nil {
		return nil, err
	}
	var err error
	item.DispatchAt, err = time.Parse(time.RFC3339Nano, due)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return &item, err
}
func (s *Store) Cancel(id string) error {
	result, err := s.db.Exec(`UPDATE remote_pending_sends SET state='cancelled',payload='',prepared=X'' WHERE id=? AND state='pending'`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("message is already submitting, cancelled or expired")
	}
	return nil
}
func (s *Store) Finish(id, state string) error {
	if state != StateUnknown && state != StateSent && state != StateFailed {
		return fmt.Errorf("invalid send outcome %q", state)
	}
	// Unknown retains immutable bytes for inspection. It can never be claimed again.
	result, err := s.db.Exec(`UPDATE remote_pending_sends SET state=?,payload=CASE WHEN ?='unknown' THEN payload ELSE '' END,prepared=CASE WHEN ?='unknown' THEN prepared ELSE X'' END WHERE id=? AND state IN ('sending','unknown')`, state, state, state, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("pending-send outcome could not be recorded")
	}
	return nil
}
func (s *Store) List() ([]Item, error) {
	rows, err := s.db.Query(`SELECT id,account_id,draft_id,payload,prepared,dispatch_at,created_at FROM remote_pending_sends WHERE state='pending' ORDER BY created_at LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
func (s *Store) Recover() error {
	// No restart sends. Interrupted submissions stay uncertain, never queued.
	_, err := s.db.Exec(`UPDATE remote_pending_sends SET state='unknown' WHERE state='sending';
 UPDATE remote_pending_sends SET state='expired',payload='',prepared=X'' WHERE state='pending';
 DELETE FROM remote_pending_sends WHERE state IN ('sent','failed','cancelled','expired');`)
	return err
}

// Uncertain returns bounded content-free IDs for helper receipt reconciliation.
func (s *Store) Uncertain() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM remote_pending_sends WHERE state='unknown' LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Connection returns the immutable connection-bound record for receipt lookup only.
func (s *Store) Connection(id string) (*Item, error) {
	return scanItem(s.db.QueryRow(`SELECT id,account_id,draft_id,payload,prepared,dispatch_at,created_at FROM remote_pending_sends WHERE id=? AND state='unknown'`, id))
}
