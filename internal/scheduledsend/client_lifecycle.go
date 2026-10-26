package scheduledsend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func upgradeClientSchema(db *sql.DB) error {
	ctx := context.Background()
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current == 2 {
		return nil
	}
	if current > 2 {
		return errors.New("Send Later client database requires a newer app")
	}
	connection, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer func() { _, _ = connection.ExecContext(ctx, "ROLLBACK") }()
	var version int
	if err := connection.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return errors.New("Send Later client database requires a newer app")
	}
	rows, err := connection.QueryContext(ctx, "PRAGMA table_info(requests)")
	if err != nil {
		return err
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var index, notNull, primary int
		var name, kind string
		var fallback any
		if err := rows.Scan(&index, &name, &kind, &notNull, &fallback, &primary); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, column := range []struct{ name, declaration string }{
		{"delivery_request", "BLOB NOT NULL DEFAULT X''"},
		{"preview", "BLOB NOT NULL DEFAULT X''"},
		{"draft_fingerprint", "TEXT NOT NULL DEFAULT ''"},
		{"cleanup_state", "TEXT NOT NULL DEFAULT 'pending'"},
	} {
		if columns[column.name] {
			continue
		}
		if _, err := connection.ExecContext(ctx, "ALTER TABLE requests ADD COLUMN "+column.name+" "+column.declaration); err != nil {
			return err
		}
	}
	if _, err := connection.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		return err
	}
	_, err = connection.ExecContext(ctx, "COMMIT")
	return err
}

// Reschedule changes only the delivery time, retaining the exact signed MIME
// and Message-ID. A higher authenticated revision replaces a pending schedule.
func (s *ClientStore) Reschedule(ctx context.Context, id string, due time.Time, client *Identity) (ClientJob, error) {
	if !due.After(time.Now()) {
		return ClientJob{}, errors.New("choose a future delivery time")
	}
	if err := s.BindIdentity(ctx, client); err != nil {
		return ClientJob{}, err
	}
	job, _, helper, _, err := s.request(ctx, id, false)
	if err != nil {
		return job, err
	}
	switch job.State {
	case "queued", "retry", "awaiting_ack", "failed":
	default:
		return job, errors.New("refresh the pending change or resolve delivery before rescheduling")
	}
	var stored []byte
	if err := s.db.QueryRowContext(ctx, "SELECT CASE WHEN LENGTH(delivery_request) > 0 THEN delivery_request ELSE request END FROM requests WHERE id = ?", id).Scan(&stored); err != nil {
		return job, err
	}
	command, err := OpenClientCommand(stored, client, helper)
	if err != nil {
		return job, err
	}
	if command.Action != "schedule" || command.Delivery == nil || command.JobID != id {
		return job, errors.New("original encrypted schedule is unavailable")
	}
	defer clear(command.Delivery.RFC822)
	command.Revision = job.Revision + 1
	command.Delivery.DueAt = due
	encoded, err := SealCommand(command, client, helper)
	if err != nil {
		return job, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE requests SET revision = revision + 1,
		delivery_request = CASE WHEN LENGTH(delivery_request) = 0 THEN request ELSE delivery_request END,
		request = ?, due_at = ?, state = 'reschedule_awaiting_ack', code = '', updated_at = ?
		WHERE id = ? AND revision = ? AND state IN ('queued','retry','awaiting_ack','failed')
		AND NOT EXISTS (SELECT 1 FROM immediate_sends WHERE immediate_sends.account_id = requests.account_id AND immediate_sends.draft_id = requests.draft_id)`,
		encoded, due.UnixMilli(), time.Now().UnixMilli(), id, job.Revision)
	if err != nil {
		return job, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return job, err
	}
	if count != 1 {
		return job, errors.New("job changed or this draft was used for Send Now; inspect delivery before rescheduling")
	}
	return s.Submit(ctx, id, client)
}

// ClientPage provides bounded cached status and encrypted local previews.
type ClientPage struct {
	Jobs         []ClientJob `json:"jobs"`
	Total        int         `json:"total"`
	KeyAvailable bool        `json:"keyAvailable"`
}

// ListPage lists active or historical jobs across one or all local accounts.
// Previews are decrypted from small local cache records; attachments are not
// loaded during normal list refreshes.
func (s *ClientStore) ListPage(ctx context.Context, accountID string, history bool, limit, offset int, client *Identity) (ClientPage, error) {
	page := ClientPage{Jobs: make([]ClientJob, 0), KeyAvailable: client != nil}
	if limit < 1 || limit > 100 || offset < 0 {
		return page, errors.New("scheduled page requires limit 1..100 and nonnegative offset")
	}
	if client != nil {
		if err := s.BindIdentity(ctx, client); err != nil {
			return page, err
		}
	}
	filter := "(state NOT IN ('sent','cancelled') OR code = 'sent_copy_failed' OR (state = 'sent' AND cleanup_state IN ('pending','retry')))"
	order := "due_at ASC, id"
	if history {
		filter = "((state = 'sent' AND code != 'sent_copy_failed' AND cleanup_state NOT IN ('pending','retry')) OR state = 'cancelled')"
		order = "updated_at DESC, id"
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM requests WHERE (? = '' OR account_id = ?) AND "+filter, accountID, accountID).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, draft_id, revision, state, due_at, code, updated_at, cleanup_state, preview FROM requests
		WHERE (? = '' OR account_id = ?) AND `+filter+" ORDER BY "+order+" LIMIT ? OFFSET ?", accountID, accountID, limit, offset)
	if err != nil {
		return page, err
	}
	var previews [][]byte
	for rows.Next() {
		var job ClientJob
		var due, updated int64
		var preview []byte
		if err := rows.Scan(&job.ID, &job.AccountID, &job.DraftID, &job.Revision, &job.State, &due, &job.Code, &updated, &job.DraftCleanup, &preview); err != nil {
			rows.Close()
			return page, err
		}
		job.DueAt, job.UpdatedAt = time.UnixMilli(due).UTC(), time.UnixMilli(updated).UTC()
		page.Jobs = append(page.Jobs, job)
		previews = append(previews, preview)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	if err := rows.Close(); err != nil {
		return page, err
	}
	if client == nil {
		return page, nil
	}
	for index := range page.Jobs {
		encoded := previews[index]
		if len(encoded) == 0 {
			encoded, _ = s.cacheLegacyPreview(ctx, page.Jobs[index].ID, client)
		}
		preview, err := openPreview(encoded, client)
		if err != nil {
			continue
		}
		page.Jobs[index].Subject, page.Jobs[index].From = preview.Subject, preview.From
		page.Jobs[index].To, page.Jobs[index].ToCount = preview.To, preview.ToCount
		page.Jobs[index].PreviewAvailable = true
	}
	return page, nil
}

func (s *ClientStore) cacheLegacyPreview(ctx context.Context, id string, client *Identity) ([]byte, error) {
	var key string
	var payload []byte
	if err := s.db.QueryRowContext(ctx, `SELECT helper_key, CASE WHEN LENGTH(delivery_request) > 0 THEN delivery_request ELSE request END FROM requests WHERE id = ?`, id).Scan(&key, &payload); err != nil {
		return nil, err
	}
	helper, err := ParsePublicKey(key)
	if err != nil {
		return nil, err
	}
	command, err := OpenClientCommand(payload, client, helper)
	if err != nil || command.Delivery == nil {
		return nil, errors.New("message preview unavailable")
	}
	defer clear(command.Delivery.RFC822)
	preview, err := deliveryPreview(command.Delivery)
	if err != nil {
		return nil, err
	}
	encoded, err := sealPreview(preview, client)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE requests SET preview = ? WHERE id = ? AND LENGTH(preview) = 0", encoded, id)
	return encoded, err
}

// SetDraftCleanup records a local cleanup result only after confirmed delivery.
func (s *ClientStore) SetDraftCleanup(ctx context.Context, id, state string) error {
	switch state {
	case "removed", "kept_changed", "waiting_sent_copy", "manual_legacy", "retry":
	default:
		return fmt.Errorf("invalid scheduled draft cleanup state")
	}
	_, err := s.db.ExecContext(ctx, "UPDATE requests SET cleanup_state = ? WHERE id = ? AND state = 'sent'", state, id)
	return err
}
