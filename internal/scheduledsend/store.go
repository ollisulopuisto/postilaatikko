package scheduledsend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// State describes durable delivery progress; Unknown always requires explicit
// reconciliation because SMTP may have accepted the message already.
type State string

const (
	Queued    State = "queued"
	Retry     State = "retry"
	Sending   State = "sending"
	Sent      State = "sent"
	Cancelled State = "cancelled"
	Failed    State = "failed"
	Unknown   State = "outcome_unknown"
)

var (
	// ErrStaleRevision rejects replayed or out-of-order commands.
	ErrStaleRevision = errors.New("scheduled command revision is stale or conflicts")
	// ErrDeliveryStarted means cancellation cannot guarantee that sending stops.
	ErrDeliveryStarted = errors.New("delivery has started; automatic cancellation or replacement is unavailable")
	// ErrStateChanged prevents a worker from overwriting recovery decisions.
	ErrStateChanged = errors.New("scheduled job state changed during delivery")
	// ErrUnpaired rejects submissions and delivery after client revocation.
	ErrUnpaired = errors.New("client is not paired or has been revoked")
	// ErrQueueFull preserves existing jobs when the private queue reaches capacity.
	ErrQueueFull = errors.New("scheduled queue capacity reached; existing jobs are preserved")
)

// Retain revision tombstones rather than silently forgetting accepted job IDs.
// These logical limits bound live ciphertext and metadata; SQLite WAL/free pages
// can occupy additional space until checkpoint/maintenance.
const maxQueueJobs = 10000
const maxQueueCiphertextBytes = 512 << 20

// SenderPermission binds one helper-local account to one permitted sender.
type SenderPermission struct {
	AccountID string `json:"accountId"`
	From      string `json:"from"`
}

// Permissions are approved locally on the helper during verified pairing.
// Clients cannot grant themselves access to other accounts or sender identities.
type Permissions struct {
	Senders []SenderPermission `json:"senders"`
}

// Status is local metadata only. A remote status transport must authenticate
// and encrypt it before returning it to a client.
type Status struct {
	ID        string    `json:"id"`
	Revision  int64     `json:"revision"`
	State     State     `json:"state"`
	DueAt     time.Time `json:"dueAt"`
	Attempts  int       `json:"attempts"`
	Code      string    `json:"code,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store owns a helper-private queue database. Plaintext delivery payloads and
// private keys are never persisted in this database.
type Store struct {
	db     *sql.DB
	helper *Identity
	wake   chan struct{}
}

// OpenStore opens a dedicated helper directory with owner-only permissions.
// It uses an independent schema to avoid coupling helper startup to app sync.
func OpenStore(directory string, helper *Identity) (*Store, error) {
	if helper == nil {
		return nil, errors.New("helper identity is required")
	}
	if err := validatePublicKey(helper.PublicKey()); err != nil {
		return nil, err
	}
	db, err := openPrivateDatabase(directory, "scheduledsend.db")
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS helper_identity (singleton INTEGER PRIMARY KEY CHECK(singleton = 1), public_key TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS paired_clients (public_key TEXT PRIMARY KEY, permissions BLOB NOT NULL);
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY, client_key TEXT NOT NULL, revision INTEGER NOT NULL,
			envelope BLOB NOT NULL, fingerprint BLOB NOT NULL,
			due_at INTEGER NOT NULL, next_attempt INTEGER NOT NULL,
			state TEXT NOT NULL CHECK(state IN ('queued','retry','sending','sent','cancelled','failed','outcome_unknown')),
			attempts INTEGER NOT NULL DEFAULT 0, code TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS jobs_due ON jobs(next_attempt) WHERE state IN ('queued', 'retry');
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize scheduled queue: %w", err)
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO helper_identity(singleton, public_key) VALUES(1, ?)", helper.PublicKey().String()); err != nil {
		db.Close()
		return nil, err
	}
	var storedKey string
	if err = db.QueryRow("SELECT public_key FROM helper_identity WHERE singleton = 1").Scan(&storedKey); err != nil || storedKey != helper.PublicKey().String() {
		db.Close()
		return nil, errors.New("queue belongs to another helper identity; do not replace its key")
	}
	return &Store{db: db, helper: helper, wake: make(chan struct{}, 1)}, nil
}

// Close releases queue database resources after the worker has stopped.
func (s *Store) Close() error { return s.db.Close() }

// PairClient records locally verified permissions. Never expose this method to
// an untrusted relay or automatically trust public keys from network requests.
func (s *Store) PairClient(ctx context.Context, key PublicKey, permissions Permissions) error {
	if err := validatePublicKey(key); err != nil {
		return err
	}
	if len(permissions.Senders) == 0 || len(permissions.Senders) > 1000 {
		return errors.New("pairing requires a key, accounts and sender addresses")
	}
	for _, sender := range permissions.Senders {
		if sender.AccountID == "" || len(sender.AccountID) > 256 {
			return errors.New("invalid pairing account ID")
		}
		parsed, err := mail.ParseAddress(sender.From)
		if err != nil || parsed.Address != sender.From || strings.ContainsAny(sender.From, "\r\n") {
			return errors.New("invalid pairing sender address")
		}
	}
	encoded, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO paired_clients(public_key, permissions) VALUES(?, ?)
		ON CONFLICT(public_key) DO UPDATE SET permissions = excluded.permissions`, key.String(), encoded)
	return err
}

// RevokeClient prevents new submissions and blocks queued deliveries from the
// client. A delivery already inside its transport cannot be recalled.
func (s *Store) RevokeClient(ctx context.Context, key PublicKey) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM paired_clients WHERE public_key = ?", key.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state = 'failed', code = 'client_authorization_revoked', updated_at = ?
		WHERE client_key = ? AND state IN ('queued','retry')`, time.Now().UTC().UnixMilli(), key.String()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) permissions(ctx context.Context, key PublicKey) (Permissions, error) {
	var permissions Permissions
	var encoded []byte
	err := s.db.QueryRowContext(ctx, "SELECT permissions FROM paired_clients WHERE public_key = ?", key.String()).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return permissions, ErrUnpaired
	}
	if err != nil {
		return permissions, fmt.Errorf("read local pairing permissions: %w", err)
	}
	if err := json.Unmarshal(encoded, &permissions); err != nil {
		return permissions, errors.New("invalid local pairing permissions")
	}
	return permissions, nil
}

func (p Permissions) allows(delivery *Delivery) bool {
	if delivery == nil {
		return true // Cancellation is further restricted to the owning key/job.
	}
	for _, sender := range p.Senders {
		if sender.AccountID == delivery.AccountID && strings.EqualFold(sender.From, delivery.From) {
			return true
		}
	}
	return false
}

// Accept verifies a paired client's encrypted schedule/cancel command and
// acknowledges only after the ciphertext and revision are durably committed.
func (s *Store) Accept(ctx context.Context, encoded []byte) (Status, error) {
	command, clientKey, permissions, err := s.authenticate(ctx, encoded)
	if err != nil {
		return Status{}, err
	}
	if command.Delivery != nil {
		defer clear(command.Delivery.RFC822)
	}
	return s.acceptCommand(ctx, encoded, command, clientKey, permissions)
}

func (s *Store) authenticate(ctx context.Context, encoded []byte) (Command, PublicKey, Permissions, error) {
	envelope, err := decodeEnvelope(encoded)
	if err != nil {
		return Command{}, PublicKey{}, Permissions{}, err
	}
	clientKey, err := ParsePublicKey(envelope.ClientKey)
	if err != nil {
		return Command{}, PublicKey{}, Permissions{}, err
	}
	permissions, err := s.permissions(ctx, clientKey)
	if err != nil {
		return Command{}, PublicKey{}, Permissions{}, err
	}
	command, _, err := openEnvelope(envelope, s.helper)
	return command, clientKey, permissions, err
}

func (s *Store) acceptCommand(ctx context.Context, encoded []byte, command Command, clientKey PublicKey, permissions Permissions) (Status, error) {
	var empty Status
	if !permissions.allows(command.Delivery) {
		return empty, errors.New("paired client is not authorized for this account or sender")
	}
	if command.Action == "status" {
		var owner string
		if err := s.db.QueryRowContext(ctx, "SELECT client_key FROM jobs WHERE id = ?", command.JobID).Scan(&owner); err != nil {
			return empty, err
		}
		if owner != clientKey.String() {
			return empty, ErrUnpaired
		}
		return s.GetStatus(ctx, command.JobID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var previous State
	var owner string
	var revision int64
	var fingerprint []byte
	digest := sha256.Sum256(encoded)
	err = tx.QueryRowContext(ctx, "SELECT client_key, revision, state, fingerprint FROM jobs WHERE id = ?", command.JobID).Scan(&owner, &revision, &previous, &fingerprint)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if err == nil {
		if owner != clientKey.String() {
			return empty, errors.New("scheduled job belongs to another client")
		}
		if command.Revision == revision && bytes.Equal(fingerprint, digest[:]) {
			if err := tx.Rollback(); err != nil {
				return empty, err
			}
			return s.GetStatus(ctx, command.JobID)
		}
		if command.Revision <= revision {
			return empty, ErrStaleRevision
		}
		if previous == Sending || previous == Sent || previous == Unknown {
			return empty, ErrDeliveryStarted
		}
	}
	var count, ciphertextBytes, previousBytes int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(LENGTH(envelope)), 0) FROM jobs").Scan(&count, &ciphertextBytes); err != nil {
		return empty, err
	}
	if previous != "" {
		if err := tx.QueryRowContext(ctx, "SELECT LENGTH(envelope) FROM jobs WHERE id = ?", command.JobID).Scan(&previousBytes); err != nil {
			return empty, err
		}
	}
	if previous == "" && count >= maxQueueJobs {
		return empty, ErrQueueFull
	}
	// An existing job can always be cancelled, even when storage is full.
	if command.Action == "schedule" && ciphertextBytes-previousBytes+int64(len(encoded)) > maxQueueCiphertextBytes {
		return empty, ErrQueueFull
	}
	state, dueAt := Cancelled, int64(0)
	if command.Action == "schedule" {
		state, dueAt = Queued, command.Delivery.DueAt.UnixMilli()
	}
	now := time.Now().UTC().UnixMilli()
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs(id, client_key, revision, envelope, fingerprint, due_at, next_attempt, state, attempts, code, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?)
		ON CONFLICT(id) DO UPDATE SET revision = excluded.revision, envelope = excluded.envelope,
		fingerprint = excluded.fingerprint, due_at = excluded.due_at, next_attempt = excluded.next_attempt,
		state = excluded.state, attempts = 0, code = '', updated_at = excluded.updated_at`,
		command.JobID, clientKey.String(), command.Revision, encoded, digest[:], dueAt, dueAt, state, now)
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return s.GetStatus(ctx, command.JobID)
}

// GetStatus returns local durable metadata for one job.
func (s *Store) GetStatus(ctx context.Context, id string) (Status, error) {
	return scanStatus(s.db.QueryRowContext(ctx, "SELECT id, revision, state, due_at, attempts, code, updated_at FROM jobs WHERE id = ?", id))
}

// ResolveUnknown records a locally verified delivery outcome. This is a helper
// administrator operation, not a relay endpoint. Only mark Failed after
// confirming that the provider did not accept the mail; a higher client
// revision may then authorize another attempt.
func (s *Store) ResolveUnknown(ctx context.Context, id string, revision int64, outcome State) error {
	if outcome != Sent && outcome != Failed {
		return errors.New("unknown delivery must be resolved as confirmed sent or confirmed not sent")
	}
	code := "manually_confirmed_not_sent"
	if outcome == Sent {
		code = "manually_confirmed_sent"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE jobs SET state = ?, code = ?, updated_at = ?,
		envelope = CASE WHEN ? = 'sent' THEN X'' ELSE envelope END
		WHERE id = ? AND revision = ? AND state = 'outcome_unknown'`,
		outcome, code, time.Now().UTC().UnixMilli(), outcome, id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrStateChanged
	}
	return nil
}

// ListStatus returns a bounded page of local queue metadata.
func (s *Store) ListStatus(ctx context.Context, limit, offset int) ([]Status, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, errors.New("queue page requires limit 1..100 and nonnegative offset")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, revision, state, due_at, attempts, code, updated_at FROM jobs ORDER BY updated_at DESC, id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := make([]Status, 0, limit)
	for rows.Next() {
		status, err := scanStatus(rows)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanStatus(row scanner) (Status, error) {
	var status Status
	var dueAt, updatedAt int64
	err := row.Scan(&status.ID, &status.Revision, &status.State, &dueAt, &status.Attempts, &status.Code, &updatedAt)
	if dueAt != 0 {
		status.DueAt = time.UnixMilli(dueAt).UTC()
	}
	status.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return status, err
}

type claimedJob struct {
	id       string
	revision int64
	attempts int
	envelope []byte
}

func (s *Store) claim(ctx context.Context, now time.Time) (claimedJob, error) {
	var job claimedJob
	err := s.db.QueryRowContext(ctx, `UPDATE jobs SET state = 'sending', attempts = attempts + 1, updated_at = ?
		WHERE id = (SELECT id FROM jobs WHERE state IN ('queued','retry') AND next_attempt <= ? ORDER BY next_attempt, id LIMIT 1)
		AND state IN ('queued','retry') RETURNING id, revision, attempts, envelope`, now.UnixMilli(), now.UnixMilli()).Scan(&job.id, &job.revision, &job.attempts, &job.envelope)
	return job, err
}

func (s *Store) finish(ctx context.Context, job claimedJob, state State, code string, next time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE jobs SET state = ?, code = ?, next_attempt = ?, updated_at = ?,
		envelope = CASE WHEN ? = 'sent' AND ? != 'sent_copy_failed' THEN X'' ELSE envelope END
		WHERE id = ? AND revision = ? AND state = 'sending'`, state, code, next.UnixMilli(), time.Now().UTC().UnixMilli(), state, code, job.id, job.revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrStateChanged
	}
	return nil
}

func (s *Store) recoverInterrupted(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET state = 'outcome_unknown', code = 'interrupted_delivery', updated_at = ?
		WHERE state = 'sending' AND updated_at < ?`, now.UnixMilli(), now.Add(-15*time.Minute).UnixMilli())
	return err
}

func (s *Store) nextDelay(ctx context.Context) (time.Duration, error) {
	var due sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT MIN(next_attempt) FROM jobs WHERE state IN ('queued','retry')").Scan(&due); err != nil {
		return 0, err
	}
	delay := 30 * time.Second // Also observes enqueueing from another process.
	if due.Valid {
		delay = min(delay, max(time.Duration(0), time.Until(time.UnixMilli(due.Int64))))
	}
	return delay, nil
}
