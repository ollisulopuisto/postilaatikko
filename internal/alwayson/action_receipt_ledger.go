package alwayson

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
)

type actionReceiptLedger struct{ db *database.DB }

func newActionReceiptLedger(db *database.DB) *actionReceiptLedger {
	return &actionReceiptLedger{db: db}
}

func (l *actionReceiptLedger) digestKey(key string, fingerprint func([]byte) []byte) string {
	if fingerprint != nil {
		return hex.EncodeToString(fingerprint([]byte(key)))
	}
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func (l *actionReceiptLedger) RecoverInterrupted(ctx context.Context) error {
	if l == nil || l.db == nil {
		return errors.New("action receipt database is unavailable")
	}
	_, err := l.db.ExecContext(ctx, `UPDATE mail_action_operations
		SET state = 'outcome_unknown', updated_at = datetime('now')
		WHERE state = 'processing'`)
	if err != nil {
		return fmt.Errorf("recover interrupted mail actions: %w", err)
	}
	_, err = l.db.ExecContext(ctx, `UPDATE mail_action_operations
		SET undo_state = 'outcome_unknown', updated_at = datetime('now') WHERE undo_state = 'undoing'`)
	if err != nil {
		return fmt.Errorf("recover interrupted mail undo: %w", err)
	}
	_, err = l.db.ExecContext(ctx, `UPDATE mail_action_undo_requests SET state = 'outcome_unknown', updated_at = datetime('now')
		WHERE state = 'reserved' AND operation_id IN (SELECT operation_id FROM mail_action_operations WHERE undo_state = 'outcome_unknown')`)
	if err != nil {
		return fmt.Errorf("recover interrupted undo receipts: %w", err)
	}
	return nil
}

func (l *actionReceiptLedger) Lookup(ctx context.Context, keyHash string) (v1.ActionReceiptRecord, bool, error) {
	var record v1.ActionReceiptRecord
	var state string
	err := l.db.QueryRowContext(ctx, `SELECT operation_id, request_hash, state, response_status, response_json
		FROM mail_action_operations WHERE idempotency_key_hash = ?`, keyHash).
		Scan(&record.OperationID, &record.RequestHash, &state, &record.Status, &record.Response)
	if errors.Is(err, sql.ErrNoRows) {
		return v1.ActionReceiptRecord{}, false, nil
	}
	if err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	record.State = v1.OperationState(state)
	return record, true, nil
}

func (l *actionReceiptLedger) Reserve(ctx context.Context, keyHash, requestHash, operationID string, action v1.ActionKind, messageCount int) (v1.ActionReceiptRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO mail_action_operations
		(idempotency_key_hash, request_hash, operation_id, state, action_kind, undo_message_count)
		VALUES (?, ?, ?, 'processing', ?, ?) ON CONFLICT(idempotency_key_hash) DO NOTHING`, keyHash, requestHash, operationID, action, messageCount)
	if err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	var record v1.ActionReceiptRecord
	var state string
	err = tx.QueryRowContext(ctx, `SELECT operation_id, request_hash, state, response_status, response_json
		FROM mail_action_operations WHERE idempotency_key_hash = ?`, keyHash).
		Scan(&record.OperationID, &record.RequestHash, &state, &record.Status, &record.Response)
	if err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	if record.RequestHash != requestHash {
		return record, false, v1.ErrActionReceiptConflict
	}
	record.State = v1.OperationState(state)
	owner := record.OperationID == operationID && record.State == "processing"
	if err := tx.Commit(); err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	return record, owner, nil
}

func (l *actionReceiptLedger) Complete(ctx context.Context, keyHash, requestHash string, action v1.ActionKind, state v1.OperationState, status int, response, undoPayload []byte) error {
	if state != v1.OperationSent && state != v1.OperationUnknown {
		return fmt.Errorf("invalid terminal action state %q", state)
	}
	result, err := l.db.ExecContext(ctx, `UPDATE mail_action_operations
		SET state = ?, response_status = ?, response_json = ?, action_kind = ?,
			undo_payload = ?, undo_state = CASE WHEN length(?) > 0 AND ? = 'sent' THEN 'available' ELSE 'unavailable' END,
			undo_expires_at = CASE WHEN length(?) > 0 AND ? = 'sent' THEN datetime('now', '+24 hours') ELSE NULL END,
			undo_message_count = CASE WHEN length(?) > 0 AND ? = 'sent' THEN ? ELSE undo_message_count END,
			updated_at = datetime('now')
		WHERE idempotency_key_hash = ? AND request_hash = ? AND state = 'processing'`,
		state, status, response, action, undoPayload, undoPayload, state, undoPayload, state,
		undoPayload, state, undoMessageCount(undoPayload), keyHash, requestHash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("action receipt is no longer processing")
	}
	return nil
}

func undoMessageCount(payload []byte) int {
	var snapshot struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &snapshot) != nil {
		return 0
	}
	return len(snapshot.Entries)
}

func (l *actionReceiptLedger) ListUndoHistory(ctx context.Context, limit int) ([]v1.ActionUndoSummary, error) {
	if err := l.ExpireUndoSnapshots(ctx); err != nil {
		return nil, err
	}
	rows, err := l.db.QueryContext(ctx, `SELECT operation_id, action_kind, created_at, COALESCE(undo_expires_at,''),
		undo_message_count, state, undo_state FROM mail_action_operations
		WHERE action_kind != '' ORDER BY created_at DESC, operation_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]v1.ActionUndoSummary, 0, limit)
	for rows.Next() {
		var item v1.ActionUndoSummary
		if err := rows.Scan(&item.OperationID, &item.Action, &item.CreatedAt, &item.UndoExpiresAt, &item.MessageCount, &item.State, &item.UndoState); err != nil {
			return nil, err
		}
		item.UndoAvailable = item.UndoState == "available"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (l *actionReceiptLedger) ExpireUndoSnapshots(ctx context.Context) error {
	_, err := l.db.ExecContext(ctx, `UPDATE mail_action_operations SET undo_state = 'expired', undo_payload = NULL
		WHERE undo_state = 'available' AND undo_expires_at <= datetime('now')`)
	return err
}

func (l *actionReceiptLedger) ClaimUndo(ctx context.Context, operationID, keyHash, requestHash string) (v1.ActionUndoRecord, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return v1.ActionUndoRecord{}, err
	}
	defer tx.Rollback()
	var mappedOperationID, existingRequestHash string
	err = tx.QueryRowContext(ctx, `SELECT operation_id, request_hash FROM mail_action_undo_requests WHERE idempotency_key_hash = ?`, keyHash).
		Scan(&mappedOperationID, &existingRequestHash)
	existingRequest := err == nil
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return v1.ActionUndoRecord{}, err
	}
	if existingRequest {
		if existingRequestHash != requestHash {
			return v1.ActionUndoRecord{}, v1.ErrActionUndoConflict
		}
		operationID = mappedOperationID
	}
	query := `SELECT operation_id, action_kind, state, undo_state, created_at, COALESCE(undo_expires_at,''), undo_message_count, undo_payload
		FROM mail_action_operations WHERE operation_id = ?`
	if operationID == "" {
		query = `SELECT operation_id, action_kind, state, undo_state, created_at, COALESCE(undo_expires_at,''), undo_message_count, undo_payload
			FROM mail_action_operations WHERE undo_state = 'available' AND undo_expires_at > datetime('now')
			ORDER BY created_at DESC, operation_id DESC LIMIT 1`
	}
	var record v1.ActionUndoRecord
	var operationState string
	var row *sql.Row
	if operationID == "" {
		row = tx.QueryRowContext(ctx, query)
	} else {
		row = tx.QueryRowContext(ctx, query, operationID)
	}
	err = row.Scan(&record.OperationID, &record.Action, &operationState, &record.UndoState,
		&record.CreatedAt, &record.ExpiresAt, &record.MessageCount, &record.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return v1.ActionUndoRecord{}, v1.ErrActionUndoUnavailable
	}
	if err != nil {
		return v1.ActionUndoRecord{}, err
	}
	record.State = v1.OperationState(operationState)
	if record.UndoState == "undone" || record.UndoState == "outcome_unknown" {
		if err := tx.Commit(); err != nil {
			return v1.ActionUndoRecord{}, err
		}
		return record, nil
	}
	if existingRequest {
		return v1.ActionUndoRecord{}, v1.ErrActionUndoBusy
	}
	if record.UndoState == "undoing" {
		return v1.ActionUndoRecord{}, v1.ErrActionUndoBusy
	}
	if record.UndoState != "available" || record.State != v1.OperationSent {
		return v1.ActionUndoRecord{}, v1.ErrActionUndoUnavailable
	}
	result, err := tx.ExecContext(ctx, `UPDATE mail_action_operations SET undo_state = 'undoing', updated_at = datetime('now')
		WHERE operation_id = ? AND undo_state = 'available' AND undo_expires_at > datetime('now') AND state = 'sent'`, record.OperationID)
	if err != nil {
		return v1.ActionUndoRecord{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return v1.ActionUndoRecord{}, err
	}
	if rows != 1 {
		return v1.ActionUndoRecord{}, v1.ErrActionUndoUnavailable
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail_action_undo_requests
		(idempotency_key_hash, request_hash, operation_id, state) VALUES (?, ?, ?, 'reserved')`, keyHash, requestHash, record.OperationID); err != nil {
		return v1.ActionUndoRecord{}, err
	}
	record.UndoState = "undoing"
	if err := tx.Commit(); err != nil {
		return v1.ActionUndoRecord{}, err
	}
	return record, nil
}

func (l *actionReceiptLedger) CompleteUndo(ctx context.Context, operationID, state string) error {
	if state != "undone" && state != "outcome_unknown" {
		return fmt.Errorf("invalid undo state %q", state)
	}
	result, err := l.db.ExecContext(ctx, `UPDATE mail_action_operations
		SET undo_state = ?, undo_payload = CASE WHEN ? = 'undone' THEN NULL ELSE undo_payload END,
			updated_at = datetime('now') WHERE operation_id = ? AND undo_state = 'undoing'`, state, state, operationID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("action undo is no longer in progress")
	}
	_, err = l.db.ExecContext(ctx, `UPDATE mail_action_undo_requests SET state = ?, updated_at = datetime('now')
		WHERE operation_id = ? AND state = 'reserved'`, state, operationID)
	if err != nil {
		return err
	}
	return nil
}
