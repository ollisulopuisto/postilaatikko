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
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin interrupted mail action recovery: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT operation_id FROM mail_action_operations WHERE state = 'processing'`)
	if err != nil {
		return fmt.Errorf("list interrupted mail actions: %w", err)
	}
	var interrupted []string
	for rows.Next() {
		var operationID string
		if err := rows.Scan(&operationID); err != nil {
			rows.Close()
			return err
		}
		interrupted = append(interrupted, operationID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, operationID := range interrupted {
		if _, err := tx.ExecContext(ctx, `UPDATE mail_action_operations SET state='outcome_unknown', updated_at=datetime('now') WHERE operation_id=? AND state='processing'`, operationID); err != nil {
			return fmt.Errorf("recover interrupted mail action: %w", err)
		}
		if err := insertActionEvent(ctx, tx, operationID, "outcome_unknown", nil); err != nil {
			return fmt.Errorf("record interrupted mail action event: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mail_action_operations
		SET undo_state = 'outcome_unknown', updated_at = datetime('now') WHERE undo_state = 'undoing'`); err != nil {
		return fmt.Errorf("recover interrupted mail undo: %w", err)
	}
	var interruptedUndos []struct {
		operationID string
		payload     []byte
	}
	undoRows, err := tx.QueryContext(ctx, `SELECT operation_id, undo_payload FROM mail_action_operations WHERE undo_state = 'outcome_unknown' AND operation_id IN (SELECT operation_id FROM mail_action_undo_requests WHERE state = 'reserved')`)
	if err != nil {
		return fmt.Errorf("list interrupted mail undos: %w", err)
	}
	for undoRows.Next() {
		var item struct {
			operationID string
			payload     []byte
		}
		if err := undoRows.Scan(&item.operationID, &item.payload); err != nil {
			undoRows.Close()
			return err
		}
		interruptedUndos = append(interruptedUndos, item)
	}
	if err := undoRows.Err(); err != nil {
		undoRows.Close()
		return err
	}
	if err := undoRows.Close(); err != nil {
		return err
	}
	for _, item := range interruptedUndos {
		if err := insertActionEvent(ctx, tx, item.operationID, "outcome_unknown", nil); err != nil {
			return fmt.Errorf("record interrupted mail undo event: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mail_action_undo_requests SET state = 'outcome_unknown', updated_at = datetime('now')
		WHERE state = 'reserved' AND operation_id IN (SELECT operation_id FROM mail_action_operations WHERE undo_state = 'outcome_unknown')`); err != nil {
		return fmt.Errorf("recover interrupted undo receipts: %w", err)
	}
	return tx.Commit()
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
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE mail_action_operations
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
	var responseData v1.ActionResponse
	if err := json.Unmarshal(response, &responseData); err != nil {
		return fmt.Errorf("decode action response for event: %w", err)
	}
	if err := insertActionEvent(ctx, tx, responseData.Receipt.OperationID, string(state), responseData.Affected); err != nil {
		return err
	}
	return tx.Commit()
}

func insertActionEvent(ctx context.Context, tx *sql.Tx, operationID, state string, affected []v1.AffectedMessageRef) error {
	if state != "sent" && state != "undone" && state != "outcome_unknown" {
		return fmt.Errorf("invalid action event state %q", state)
	}
	if affected == nil {
		affected = []v1.AffectedMessageRef{}
	}
	payload, err := json.Marshal(affected)
	if err != nil {
		return fmt.Errorf("encode affected action references: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mail_action_events (operation_id, state, affected_json)
		VALUES (?, ?, ?) ON CONFLICT(operation_id, state) DO NOTHING`, operationID, state, payload)
	return err
}

func (l *actionReceiptLedger) ListActionEvents(ctx context.Context, clientID string, limit int) (v1.ActionEventPage, error) {
	if clientID == "" || limit < 1 || limit > 100 {
		return v1.ActionEventPage{}, errors.New("client ID and bounded event limit are required")
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return v1.ActionEventPage{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mail_action_events WHERE created_at < datetime('now', '-90 days')
		AND sequence < (SELECT COALESCE(MAX(sequence),0) FROM mail_action_events)`); err != nil {
		return v1.ActionEventPage{}, err
	}
	var cursor int64
	err = tx.QueryRowContext(ctx, `SELECT last_sequence FROM mail_action_event_cursors WHERE client_id=?`, clientID).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return v1.ActionEventPage{}, err
	}
	var oldest, newest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(sequence), MAX(sequence) FROM mail_action_events`).Scan(&oldest, &newest); err != nil {
		return v1.ActionEventPage{}, err
	}
	page := v1.ActionEventPage{Events: []v1.ActionEvent{}, Cursor: cursor}
	if oldest.Valid && cursor < oldest.Int64-1 {
		page.ResetRequired = true
		page.Cursor = newest.Int64
		return page, tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT sequence, operation_id, state, affected_json, created_at
		FROM mail_action_events WHERE sequence > ? ORDER BY sequence LIMIT ?`, cursor, limit)
	if err != nil {
		return v1.ActionEventPage{}, err
	}
	for rows.Next() {
		var item v1.ActionEvent
		var affected []byte
		if err := rows.Scan(&item.Sequence, &item.OperationID, &item.State, &affected, &item.CreatedAt); err != nil {
			rows.Close()
			return v1.ActionEventPage{}, err
		}
		if err := json.Unmarshal(affected, &item.Affected); err != nil {
			rows.Close()
			return v1.ActionEventPage{}, err
		}
		page.Events = append(page.Events, item)
		page.Cursor = item.Sequence
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return v1.ActionEventPage{}, err
	}
	if err := rows.Close(); err != nil {
		return v1.ActionEventPage{}, err
	}
	return page, tx.Commit()
}

func (l *actionReceiptLedger) AcknowledgeActionEvents(ctx context.Context, clientID string, sequence int64) error {
	if clientID == "" || sequence < 0 {
		return errors.New("client ID and non-negative sequence are required")
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int64
	err = tx.QueryRowContext(ctx, `SELECT last_sequence FROM mail_action_event_cursors WHERE client_id=?`, clientID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return err
	}
	if sequence < current {
		return errors.New("action-event cursor cannot move backwards")
	}
	var maximum int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM mail_action_events`).Scan(&maximum); err != nil {
		return err
	}
	if sequence > maximum {
		return errors.New("action-event cursor exceeds latest sequence")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail_action_event_cursors(client_id,last_sequence) VALUES(?,?)
		ON CONFLICT(client_id) DO UPDATE SET last_sequence=excluded.last_sequence, updated_at=datetime('now')`, clientID, sequence); err != nil {
		return err
	}
	return tx.Commit()
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
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT undo_payload FROM mail_action_operations WHERE operation_id=?`, operationID).Scan(&payload); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE mail_action_operations
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
	if _, err = tx.ExecContext(ctx, `UPDATE mail_action_undo_requests SET state = ?, updated_at = datetime('now')
		WHERE operation_id = ? AND state = 'reserved'`, state, operationID); err != nil {
		return err
	}
	var affected []v1.AffectedMessageRef
	if state == "undone" {
		var snapshot struct {
			Entries []struct {
				AccountID string `json:"accountId"`
				MessageID string `json:"messageId"`
				FolderID  string `json:"folderId"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return err
		}
		for _, entry := range snapshot.Entries {
			affected = append(affected, v1.AffectedMessageRef{AccountID: entry.AccountID, MessageID: entry.MessageID, FolderID: entry.FolderID})
		}
	}
	if err := insertActionEvent(ctx, tx, operationID, state, affected); err != nil {
		return err
	}
	return tx.Commit()
}
