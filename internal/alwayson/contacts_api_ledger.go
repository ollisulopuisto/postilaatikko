package alwayson

import (
	"context"

	"github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
)

// contactsMutationLedger namespaces contact mutation idempotency keys inside
// the shared account_api_idempotency table so a mail-mutation key can never
// replay a contact write. A NULL stored response means the outcome is
// unknown (interrupted mid-mutation); the route surfaces that as a conflict
// instead of re-executing.
type contactsMutationLedger struct {
	inner *accountMutationLedger
}

func newContactsMutationLedger(db *database.DB) *contactsMutationLedger {
	return &contactsMutationLedger{inner: newAccountMutationLedger(db)}
}

func (l *contactsMutationLedger) Reserve(ctx context.Context, key, hash string) ([]byte, int, error) {
	return l.inner.Reserve(ctx, "contacts:"+key, hash)
}

func (l *contactsMutationLedger) Complete(ctx context.Context, key, hash string, status int, response []byte) error {
	return l.inner.Complete(ctx, "contacts:"+key, hash, status, response)
}

var _ v1.ContactsMutationLedger = (*contactsMutationLedger)(nil)
