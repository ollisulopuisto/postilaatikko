package alwayson

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
)

type accountMutationLedger struct {
	db *database.DB
	mu sync.Mutex
}

func newAccountMutationLedger(db *database.DB) *accountMutationLedger {
	return &accountMutationLedger{db: db}
}

func (l *accountMutationLedger) Reserve(ctx context.Context, key, hash string) ([]byte, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var existingHash string
	var status int
	var response []byte
	err := l.db.QueryRowContext(ctx, `SELECT request_hash, response_status, response_json FROM account_api_idempotency WHERE idempotency_key = ?`, key).Scan(&existingHash, &status, &response)
	if errors.Is(err, sql.ErrNoRows) {
		_, err := l.db.ExecContext(ctx, `INSERT INTO account_api_idempotency (idempotency_key, request_hash) VALUES (?, ?)`, key, hash)
		return nil, 0, err
	}
	if err != nil {
		return nil, 0, err
	}
	if existingHash != hash {
		return nil, 0, v1.ErrAccountMutationKeyConflict
	}
	if response == nil {
		return nil, 0, v1.ErrAccountMutationInProgress
	}
	return response, status, nil
}

func (l *accountMutationLedger) Complete(ctx context.Context, key, hash string, status int, response []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := l.db.ExecContext(ctx, `UPDATE account_api_idempotency SET response_status = ?, response_json = ?, updated_at = datetime('now') WHERE idempotency_key = ? AND request_hash = ? AND response_json IS NULL`, status, response, key, hash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return v1.ErrAccountMutationInProgress
	}
	return nil
}
