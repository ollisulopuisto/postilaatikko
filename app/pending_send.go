package app

// 30-second undo-send queue (user-approved design):
//
//   - Clicking send queues the message here with a dispatch time ~30s out
//     and emits "pending-send:queued" so the main window shows the
//     countdown. The composer closes as usual; the DRAFT STAYS in the
//     local store until the send actually succeeds (the normal send path
//     deletes it), which is what makes cancel = restore-trivially-safe.
//   - The frontend main window owns the timer and dispatches through the
//     identical SendMessageWithDraftGuard call a direct send would use.
//   - Cancel deletes the pending row; the draft is then reopened.
//   - A restart never auto-sends: ExpirePendingSends marks leftover rows
//     expired at startup and the draft simply reappears in Drafts.
//
// Concurrency: QueuePendingSend rejects a second pending send for the same
// draft (Cmd-Enter pressed twice), so the safety window can never be
// bypassed by re-sending. Dispatch-now (Cmd-Shift-Enter) only shortens the
// wait for the EXISTING row.

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PendingSend is the durable record of one queued send.
type PendingSend struct {
	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	DraftID    string `json:"draftId"`
	Payload    string `json:"payload"` // serialized smtp.ComposeMessage
	DispatchAt string `json:"dispatchAt"`
	State      string `json:"state"` // pending | expired
	CreatedAt  string `json:"createdAt"`
}

// PendingSendDelay is the user-approved safety window.
const PendingSendDelay = 30 * time.Second

// QueuePendingSend stores one pending send and notifies the main window.
// payload is the serialized smtp.ComposeMessage exactly as SendMessage
// would receive it. Returns the pending-send ID.
func (a *App) QueuePendingSend(accountID, draftID, payload string) (string, error) {
	if accountID == "" || payload == "" {
		return "", fmt.Errorf("pending send: account and payload are required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	dispatchAt := now.Add(PendingSendDelay)

	tx, err := a.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if draftID != "" {
		var n int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM pending_sends WHERE draft_id = ? AND state = 'pending'`, draftID).Scan(&n); err != nil {
			return "", err
		}
		if n > 0 {
			return "", fmt.Errorf("pending send: this draft is already queued to send")
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO pending_sends (id, account_id, draft_id, payload, dispatch_at, state, created_at)
		 VALUES (?,?,?,?,?,'pending',?)`,
		id, accountID, draftID, payload,
		dispatchAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	wailsRuntime.EventsEmit(a.ctx, "pending-send:queued", PendingSend{
		ID: id, AccountID: accountID, DraftID: draftID, Payload: payload,
		DispatchAt: dispatchAt.Format(time.RFC3339Nano), State: "pending",
		CreatedAt: now.Format(time.RFC3339Nano),
	})
	return id, nil
}

// CancelPendingSend removes a pending send that has not dispatched. The
// frontend only calls this before its timer fires; after dispatch begins
// the row is already gone and cancel reports an error the UI must show
// truthfully ("already sending").
func (a *App) CancelPendingSend(id string) error {
	res, err := a.db.Exec(`DELETE FROM pending_sends WHERE id = ? AND state = 'pending'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pending send: %q is not pending (already sent or cancelled)", id)
	}
	wailsRuntime.EventsEmit(a.ctx, "pending-send:cancelled", id)
	return nil
}

// CompletePendingSend removes the row after a successful dispatch. Called
// by the frontend only after the send call returned without error.
func (a *App) CompletePendingSend(id string) error {
	_, err := a.db.Exec(`DELETE FROM pending_sends WHERE id = ?`, id)
	return err
}

// ListPendingSends returns all pending rows (main-window recovery on focus
// or startup before expiry).
func (a *App) ListPendingSends() ([]*PendingSend, error) {
	rows, err := a.db.Query(
		`SELECT id, account_id, draft_id, payload, dispatch_at, state, created_at
		 FROM pending_sends WHERE state = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*PendingSend{}
	for rows.Next() {
		p := &PendingSend{}
		if err := rows.Scan(&p.ID, &p.AccountID, &p.DraftID, &p.Payload, &p.DispatchAt, &p.State, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ExpirePendingSends marks every leftover pending row expired. Called at
// startup: a restart NEVER auto-sends; the draft reappears in Drafts.
func (a *App) ExpirePendingSends() error {
	_, err := a.db.Exec(`UPDATE pending_sends SET state = 'expired' WHERE state = 'pending'`)
	return err
}
