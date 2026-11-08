package alwayson

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

func (l *actionReceiptLedger) Reserve(ctx context.Context, keyHash, requestHash, operationID string) (v1.ActionReceiptRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return v1.ActionReceiptRecord{}, false, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO mail_action_operations
		(idempotency_key_hash, request_hash, operation_id, state)
		VALUES (?, ?, ?, 'processing') ON CONFLICT(idempotency_key_hash) DO NOTHING`, keyHash, requestHash, operationID)
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

func (l *actionReceiptLedger) Complete(ctx context.Context, keyHash, requestHash string, state v1.OperationState, status int, response []byte) error {
	if state != v1.OperationSent && state != v1.OperationUnknown {
		return fmt.Errorf("invalid terminal action state %q", state)
	}
	result, err := l.db.ExecContext(ctx, `UPDATE mail_action_operations
		SET state = ?, response_status = ?, response_json = ?, updated_at = datetime('now')
		WHERE idempotency_key_hash = ? AND request_hash = ? AND state = 'processing'`, state, status, response, keyHash, requestHash)
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
