package scheduledsend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ClientConfig pins a verified helper key and explicitly maps one local account
// to the corresponding sending account configured on the helper.
type ClientConfig struct {
	AccountID       string `json:"accountId"`
	Endpoint        string `json:"endpoint"`
	HelperKey       string `json:"helperKey"`
	HelperAccountID string `json:"helperAccountId"`
}

// ClientJob is local progress metadata; its MIME payload stays encrypted.
type ClientJob struct {
	ID               string    `json:"id"`
	AccountID        string    `json:"accountId"`
	DraftID          string    `json:"draftId"`
	Revision         int64     `json:"revision"`
	State            string    `json:"state"`
	DueAt            time.Time `json:"dueAt"`
	Code             string    `json:"code"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Subject          string    `json:"subject,omitempty"`
	From             string    `json:"from,omitempty"`
	To               string    `json:"to,omitempty"`
	ToCount          int       `json:"toCount,omitempty"`
	DraftCleanup     string    `json:"draftCleanup"`
	DraftFingerprint string    `json:"-"`
	PreviewAvailable bool      `json:"previewAvailable"`
}

// ClientStore persists ciphertext before submission so network failures can be
// retried with identical bytes without producing conflicting revisions.
type ClientStore struct{ db *sql.DB }

// OpenClientStore opens the installation's independent private client queue.
func OpenClientStore(directory string) (*ClientStore, error) {
	db, err := openPrivateDatabase(directory, "client.db")
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS configs (
		account_id TEXT PRIMARY KEY, endpoint TEXT NOT NULL, helper_key TEXT NOT NULL, helper_account_id TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY, account_id TEXT NOT NULL, draft_id TEXT NOT NULL, revision INTEGER NOT NULL,
		endpoint TEXT NOT NULL, helper_key TEXT NOT NULL, request BLOB NOT NULL,
		due_at INTEGER NOT NULL, state TEXT NOT NULL, code TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL);
		CREATE UNIQUE INDEX IF NOT EXISTS requests_draft ON requests(account_id, draft_id) WHERE draft_id != '' AND state NOT IN ('cancelled','failed');
		CREATE TABLE IF NOT EXISTS client_identity(singleton INTEGER PRIMARY KEY CHECK(singleton = 1), public_key TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS immediate_sends (
			account_id TEXT NOT NULL, draft_id TEXT NOT NULL, attempted_at INTEGER NOT NULL,
			PRIMARY KEY(account_id, draft_id));`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := upgradeClientSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &ClientStore{db: db}, nil
}

// Close releases the installation's client queue database.
func (s *ClientStore) Close() error { return s.db.Close() }

// SaveConfig pins user-verified helper information. Changing a config does not
// redirect existing encrypted jobs; each request snapshots its original helper.
func (s *ClientStore) SaveConfig(ctx context.Context, config ClientConfig) error {
	if config.AccountID == "" || config.HelperAccountID == "" || len(config.AccountID) > 256 || len(config.HelperAccountID) > 256 {
		return errors.New("local and helper account IDs are required")
	}
	if _, err := ParsePublicKey(config.HelperKey); err != nil {
		return err
	}
	if err := ValidateEndpoint(config.Endpoint); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO configs(account_id, endpoint, helper_key, helper_account_id) VALUES(?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET endpoint = excluded.endpoint, helper_key = excluded.helper_key, helper_account_id = excluded.helper_account_id`,
		config.AccountID, config.Endpoint, config.HelperKey, config.HelperAccountID)
	return err
}

// Config returns the configured helper for a local account.
func (s *ClientStore) Config(ctx context.Context, accountID string) (ClientConfig, error) {
	config := ClientConfig{AccountID: accountID}
	err := s.db.QueryRowContext(ctx, "SELECT endpoint, helper_key, helper_account_id FROM configs WHERE account_id = ?", accountID).Scan(&config.Endpoint, &config.HelperKey, &config.HelperAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return config, nil
	}
	return config, err
}

// Schedule stores ciphertext durably before attempting helper submission. On a
// network error it still returns the local job ID so the UI can retry/query it.
func (s *ClientStore) Schedule(ctx context.Context, accountID, draftID, draftFingerprint string, delivery Delivery, client *Identity) (ClientJob, error) {
	if draftID == "" {
		return ClientJob{}, errors.New("save the draft before scheduling")
	}
	if err := s.BindIdentity(ctx, client); err != nil {
		return ClientJob{}, err
	}
	config, err := s.Config(ctx, accountID)
	if err != nil {
		return ClientJob{}, err
	}
	helper, err := ParsePublicKey(config.HelperKey)
	if err != nil {
		return ClientJob{}, errors.New("configure and pair the Send Later helper first")
	}
	delivery.AccountID = config.HelperAccountID
	command := Command{Version: ProtocolVersion, JobID: uuid.NewString(), Revision: 1, Action: "schedule", Delivery: &delivery}
	encoded, err := SealCommand(command, client, helper)
	if err != nil {
		return ClientJob{}, err
	}
	preview, err := deliveryPreview(&delivery)
	if err != nil {
		return ClientJob{}, err
	}
	sealedPreview, err := sealPreview(preview, client)
	if err != nil {
		return ClientJob{}, err
	}
	inserted, err := s.db.ExecContext(ctx, `INSERT INTO requests(id, account_id, draft_id, revision, endpoint, helper_key, request, due_at, state, updated_at, preview, draft_fingerprint)
		SELECT ?, ?, ?, 1, ?, ?, ?, ?, 'awaiting_ack', ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM immediate_sends WHERE account_id = ? AND draft_id = ?)`, command.JobID, accountID, draftID, config.Endpoint, config.HelperKey, encoded, delivery.DueAt.UnixMilli(), time.Now().UnixMilli(), sealedPreview, draftFingerprint, accountID, draftID)
	if err != nil {
		if existing, findErr := s.FindDraft(ctx, accountID, draftID); findErr == nil {
			return existing, errors.New("this draft already has a scheduled job; inspect it before scheduling again")
		}
		return ClientJob{}, err
	}
	count, err := inserted.RowsAffected()
	if err != nil {
		return ClientJob{}, err
	}
	if count != 1 {
		return ClientJob{}, errors.New("this draft has already been used for Send Now; inspect delivery before scheduling a new draft")
	}
	job, submitErr := s.Submit(ctx, command.JobID, client)
	if job.ID == "" {
		job = ClientJob{ID: command.JobID, AccountID: accountID, DraftID: draftID, Revision: 1, State: "awaiting_ack", DueAt: delivery.DueAt, UpdatedAt: time.Now().UTC()}
	}
	return job, submitErr
}

// BeginImmediateSend atomically arbitrates Send Now against scheduling in other
// windows/processes. Keep this small durable marker even after a failed send:
// ordinary SMTP errors can be ambiguous, so switching that draft to Send Later
// must not silently cause another delivery. Ordinary Send Now retry behavior is
// unchanged; this marker is not an exactly-once SMTP guarantee.
func (s *ClientStore) BeginImmediateSend(ctx context.Context, accountID, draftID string) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO immediate_sends(account_id, draft_id, attempted_at)
		SELECT ?, ?, ? WHERE NOT EXISTS (
			SELECT 1 FROM requests WHERE account_id = ? AND draft_id = ? AND state NOT IN ('cancelled','failed'))
		ON CONFLICT(account_id, draft_id) DO UPDATE SET attempted_at = excluded.attempted_at`,
		accountID, draftID, time.Now().UnixMilli(), accountID, draftID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("this draft already has a Send Later job; inspect or cancel it before sending again")
	}
	return nil
}

// Submit retries the exact durable request bytes and records verified status.
func (s *ClientStore) Submit(ctx context.Context, id string, client *Identity) (ClientJob, error) {
	if err := s.BindIdentity(ctx, client); err != nil {
		return ClientJob{}, err
	}
	job, endpoint, helper, encoded, err := s.request(ctx, id, true)
	if err != nil {
		return job, err
	}
	if len(encoded) == 0 {
		return job, errors.New("completed job has no request to retry")
	}
	status, err := SendEnvelope(ctx, endpoint, encoded, client, helper)
	if err != nil {
		var remote *RemoteError
		if (job.State == "cancellation_awaiting_ack" || job.State == "reschedule_awaiting_ack") && errors.As(err, &remote) && (remote.Code == "delivery_started" || remote.Code == "stale_revision") {
			// An authenticated rejection means this cancellation revision
			// was never accepted. Query with a fresh nonce, then reconcile
			// even if the helper's actual revision is the earlier schedule.
			actual, queryErr := queryHelperStatus(ctx, job, endpoint, helper, client)
			if queryErr == nil && actual.ID == job.ID && actual.Revision > 0 {
				result, writeErr := s.db.ExecContext(ctx, `UPDATE requests SET revision = ?, state = ?, code = ?, request = X'', due_at = CASE WHEN ? != 0 THEN ? ELSE due_at END, updated_at = ?
					WHERE id = ? AND revision = ? AND state IN ('cancellation_awaiting_ack','reschedule_awaiting_ack')`,
					actual.Revision, actual.State, actual.Code, statusDueMillis(actual), statusDueMillis(actual), time.Now().UnixMilli(), job.ID, job.Revision)
				if writeErr != nil {
					return job, writeErr
				}
				count, countErr := result.RowsAffected()
				if countErr != nil || count != 1 {
					return job, ErrStateChanged
				}
				job.Revision, job.State, job.Code = actual.Revision, string(actual.State), actual.Code
				if !actual.DueAt.IsZero() {
					job.DueAt = actual.DueAt
				}
			}
		}
		return job, err
	}
	return s.record(ctx, job, status)
}

// Refresh queries durable helper status using a freshly encrypted request,
// preventing replay of a receipt from an older query.
func (s *ClientStore) Refresh(ctx context.Context, id string, client *Identity) (ClientJob, error) {
	if err := s.BindIdentity(ctx, client); err != nil {
		return ClientJob{}, err
	}
	job, endpoint, helper, _, err := s.request(ctx, id, false)
	if err != nil {
		return job, err
	}
	status, err := queryHelperStatus(ctx, job, endpoint, helper, client)
	if err != nil {
		return job, err
	}
	return s.record(ctx, job, status)
}

func queryHelperStatus(ctx context.Context, job ClientJob, endpoint string, helper PublicKey, client *Identity) (Status, error) {
	encoded, err := SealCommand(Command{Version: ProtocolVersion, JobID: job.ID, Revision: job.Revision, Action: "status"}, client, helper)
	if err != nil {
		return Status{}, err
	}
	return SendEnvelope(ctx, endpoint, encoded, client, helper)
}

// Cancel persists a higher-revision cancellation before submitting it. If the
// helper is offline, the UI must report cancellation as awaiting confirmation.
func (s *ClientStore) Cancel(ctx context.Context, id string, client *Identity) (ClientJob, error) {
	if err := s.BindIdentity(ctx, client); err != nil {
		return ClientJob{}, err
	}
	job, _, helper, _, err := s.request(ctx, id, false)
	if err != nil {
		return job, err
	}
	encoded, err := SealCommand(Command{Version: ProtocolVersion, JobID: id, Revision: job.Revision + 1, Action: "cancel"}, client, helper)
	if err != nil {
		return job, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE requests SET revision = revision + 1,
		delivery_request = CASE WHEN LENGTH(delivery_request) = 0 THEN request ELSE delivery_request END,
		request = ?, state = 'cancellation_awaiting_ack', code = '', updated_at = ?
		WHERE id = ? AND revision = ?`, encoded, time.Now().UnixMilli(), id, job.Revision)
	if err != nil {
		return job, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return job, ErrStaleRevision
	}
	return s.Submit(ctx, id, client)
}

func (s *ClientStore) request(ctx context.Context, id string, includePayload bool) (ClientJob, string, PublicKey, []byte, error) {
	var job ClientJob
	var endpoint, key string
	var encoded []byte
	var due, updated int64
	payload := "X''"
	if includePayload {
		payload = "request"
	}
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, draft_id, revision, state, due_at, code, updated_at, cleanup_state, draft_fingerprint, endpoint, helper_key, `+payload+` FROM requests WHERE id = ?`, id).
		Scan(&job.ID, &job.AccountID, &job.DraftID, &job.Revision, &job.State, &due, &job.Code, &updated, &job.DraftCleanup, &job.DraftFingerprint, &endpoint, &key, &encoded)
	job.DueAt, job.UpdatedAt = time.UnixMilli(due).UTC(), time.UnixMilli(updated).UTC()
	if err != nil {
		return job, endpoint, PublicKey{}, nil, err
	}
	helper, err := ParsePublicKey(key)
	return job, endpoint, helper, encoded, err
}

func (s *ClientStore) record(ctx context.Context, job ClientJob, status Status) (ClientJob, error) {
	if status.ID != job.ID || status.Revision < job.Revision {
		return job, errors.New("helper has not confirmed the pending request revision")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE requests SET revision = ?, state = ?, code = ?, updated_at = ?,
		request = CASE WHEN ? IN ('sent','cancelled') AND ? != 'sent_copy_failed' THEN X'' ELSE request END
		, delivery_request = CASE WHEN ? IN ('sent','cancelled') AND ? != 'sent_copy_failed' THEN X'' ELSE delivery_request END,
		due_at = CASE WHEN ? != 0 THEN ? ELSE due_at END
		WHERE id = ? AND revision = ? AND state = ? AND updated_at = ?`, status.Revision, status.State, status.Code, time.Now().UnixMilli(), status.State, status.Code, status.State, status.Code, statusDueMillis(status), statusDueMillis(status), job.ID, job.Revision, job.State, job.UpdatedAt.UnixMilli())
	if err != nil {
		return job, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return job, ErrStateChanged
	}
	job.Revision, job.State, job.Code, job.UpdatedAt = status.Revision, string(status.State), status.Code, time.Now().UTC()
	if !status.DueAt.IsZero() {
		job.DueAt = status.DueAt
	}
	return job, nil
}

func statusDueMillis(status Status) int64 {
	if status.DueAt.IsZero() {
		return 0
	}
	return status.DueAt.UnixMilli()
}

// List returns at most 100 local jobs without loading encrypted MIME payloads.
func (s *ClientStore) List(ctx context.Context, accountID string) ([]ClientJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, draft_id, revision, state, due_at, code, updated_at FROM requests
		WHERE account_id = ? ORDER BY updated_at DESC LIMIT 100`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]ClientJob, 0)
	for rows.Next() {
		var job ClientJob
		var due, updated int64
		if err := rows.Scan(&job.ID, &job.AccountID, &job.DraftID, &job.Revision, &job.State, &due, &job.Code, &updated); err != nil {
			return nil, err
		}
		job.DueAt, job.UpdatedAt = time.UnixMilli(due).UTC(), time.UnixMilli(updated).UTC()
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scheduled jobs: %w", err)
	}
	return jobs, nil
}

// FindDraft finds a draft's active/previously sent job to prevent a second
// scheduling action or an unnoticed Send Now from a reopened draft.
func (s *ClientStore) FindDraft(ctx context.Context, accountID, draftID string) (ClientJob, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM requests WHERE account_id = ? AND draft_id = ?
		AND state NOT IN ('cancelled','failed') ORDER BY updated_at DESC LIMIT 1`, accountID, draftID).Scan(&id)
	if err != nil {
		return ClientJob{}, err
	}
	job, _, _, _, err := s.request(ctx, id, false)
	return job, err
}

// PinnedIdentity returns the client key originally bound to local jobs.
func (s *ClientStore) PinnedIdentity(ctx context.Context) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx, "SELECT public_key FROM client_identity WHERE singleton = 1").Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return key, err
}

// BindIdentity prevents a missing/replaced device key from silently abandoning
// jobs encrypted under an older paired client identity.
func (s *ClientStore) BindIdentity(ctx context.Context, identity *Identity) error {
	if identity == nil {
		return errors.New("Send Later client identity is required")
	}
	if err := validatePublicKey(identity.PublicKey()); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO client_identity(singleton, public_key) VALUES(1, ?)", identity.PublicKey().String()); err != nil {
		return err
	}
	pinned, err := s.PinnedIdentity(ctx)
	if err != nil {
		return err
	}
	if pinned != identity.PublicKey().String() {
		return errors.New("device key differs from queued jobs; recover the original key or reconcile them on the Studio")
	}
	return nil
}
