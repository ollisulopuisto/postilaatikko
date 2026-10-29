package undo

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/hkdb/aerion/internal/logging"
	"github.com/rs/zerolog"
)

func storeLog() *zerolog.Logger {
	l := logging.WithComponent("undo")
	return &l
}

// OperationStore is the durable half of the mailbox undo history. Rows live
// in mailbox_undo_commands (migration 59), one per command in global push
// order. Retention is derived at load time from created_at plus the stack
// timeout; Sweep removes rows that can never become loadable again.
//
// All methods are best-effort by contract of the caller: a persistence error
// must never fail the user's action or their undo.
type OperationStore struct {
	db *sql.DB
}

// NewOperationStore wraps the app database.
func NewOperationStore(db *sql.DB) *OperationStore {
	return &OperationStore{db: db}
}

// persistedRow mirrors one mailbox_undo_commands row.
type persistedRow struct {
	OperationID string
	Action      string
	MessageIDs  []string
	Type        string
	Payload     []byte
	CreatedAt   time.Time
}

// persistCommand inserts one pushed command. createdAt is the command's own
// creation time so expiry matches in-memory semantics exactly.
func (s *OperationStore) persistCommand(operationID, action string, messageIDs []string, cmd Command) {
	if s == nil {
		return
	}
	cmdType, payload, ok := marshalCommand(cmd)
	if !ok {
		// Non-durable kinds stay memory-only.
		return
	}
	ids, err := json.Marshal(messageIDs)
	if err != nil {
		ids = []byte("[]")
	}
	if _, err := s.db.Exec(
		`INSERT INTO mailbox_undo_commands (operation_id, action, message_ids, type, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		operationID, action, string(ids), cmdType, string(payload),
		cmd.CreatedAt().UTC().Format("2006-01-02 15:04:05.000000000")); err != nil {
		storeLog().Warn().Err(err).Str("operationId", operationID).Msg("UNDO STORE SAVE ERROR")
	}
}

// removeOperation deletes every row of one operation (claimed, discarded, or
// cleared). Missing rows are fine.
func (s *OperationStore) removeOperation(operationID string) {
	if s == nil {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM mailbox_undo_commands WHERE operation_id = ?`, operationID); err != nil {
		storeLog().Warn().Err(err).Str("operationId", operationID).Msg("UNDO STORE REMOVE ERROR")
	}
}

// removeAll wipes the whole table (Stack.Clear).
func (s *OperationStore) removeAll() {
	if s == nil {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM mailbox_undo_commands`); err != nil {
		storeLog().Warn().Err(err).Msg("UNDO STORE CLEAR ERROR")
	}
}

// Sweep deletes rows older than the retention window. Returns the count for
// storage-growth observability (the user asked to watch how big the history
// gets before extending retention).
func (s *OperationStore) Sweep(now time.Time, timeout time.Duration) int {
	if s == nil {
		return 0
	}
	cutoff := now.Add(-timeout).UTC().Format("2006-01-02 15:04:05.000000000")
	res, err := s.db.Exec(`DELETE FROM mailbox_undo_commands WHERE created_at <= ?`, cutoff)
	if err != nil {
		storeLog().Warn().Err(err).Msg("UNDO STORE SWEEP ERROR")
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// Count reports how many rows the history currently holds.
func (s *OperationStore) Count() int {
	if s == nil {
		return 0
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM mailbox_undo_commands`).Scan(&n); err != nil {
		return 0
	}
	return n
}

// loadActive returns every row still inside the retention window, oldest
// first (global push order), capped to the newest limit rows.
func (s *OperationStore) loadActive(now time.Time, timeout time.Duration, limit int) []persistedRow {
	if s == nil {
		return nil
	}
	cutoff := now.Add(-timeout).UTC().Format("2006-01-02 15:04:05.000000000")
	rows, err := s.db.Query(
		`SELECT operation_id, action, message_ids, type, payload, created_at
		 FROM mailbox_undo_commands WHERE created_at > ? ORDER BY seq`, cutoff)
	if err != nil {
		storeLog().Warn().Err(err).Msg("UNDO STORE LOAD ERROR")
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []persistedRow
	for rows.Next() {
		var r persistedRow
		var ids, created string
		if err := rows.Scan(&r.OperationID, &r.Action, &ids, &r.Type, &r.Payload, &created); err != nil {
			storeLog().Warn().Err(err).Msg("UNDO STORE SCAN ERROR")
			continue
		}
		_ = json.Unmarshal([]byte(ids), &r.MessageIDs)
		t, err := time.Parse("2006-01-02 15:04:05.000000000", created)
		if err != nil {
			continue
		}
		r.CreatedAt = t
		out = append(out, r)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
