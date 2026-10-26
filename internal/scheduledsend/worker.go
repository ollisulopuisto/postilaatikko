package scheduledsend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Transport delivers an already prepared immutable MIME message using the
// helper's local credentials. It must honor context deadlines and must not
// retain the payload after returning. A Sent-folder save failure must not be
// reported as a delivery failure once the provider has accepted the message.
type Transport interface {
	Deliver(context.Context, Delivery) error
}

// FailureKind is the transport's explicit knowledge about provider acceptance.
type FailureKind int

const (
	// OutcomeUnknown covers a lost response during/after message submission.
	OutcomeUnknown FailureKind = iota
	// SafeRetry means the transport knows the provider did not accept the mail.
	SafeRetry
	// PermanentFailure means retrying unchanged input cannot succeed.
	PermanentFailure
	// AuthorizationRequired pauses the job until the user reauthorizes locally.
	AuthorizationRequired
	// AcceptedWithoutSentCopy means delivery succeeded but the recoverable
	// post-send IMAP copy failed. It must never retry SMTP delivery.
	AcceptedWithoutSentCopy
)

// DeliveryError classifies failures without persisting potentially sensitive
// provider error strings in queue status or exposing them to a relay.
type DeliveryError struct {
	Kind FailureKind
	Err  error
}

// Error returns a generic description safe for local operational logging.
func (e *DeliveryError) Error() string {
	if e.Kind == AcceptedWithoutSentCopy {
		return "scheduled delivery accepted; Sent copy failed"
	}
	return "scheduled delivery failed"
}

// Unwrap permits local transport diagnostics without storing raw errors.
func (e *DeliveryError) Unwrap() error { return e.Err }

// Worker processes due jobs serially with a bounded in-memory payload. Atomic
// queue claims prevent two workers from submitting the same job concurrently.
type Worker struct {
	Store     *Store
	Transport Transport
}

// Run waits for each due job, wakes on accepted local commands, and processes
// missed due times after restart. It stops on storage errors to avoid sending
// without durable progress. The caller owns helper lifecycle and shutdown.
func (w *Worker) Run(ctx context.Context) error {
	if w.Store == nil || w.Transport == nil {
		return errors.New("scheduled worker requires a queue and transport")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.Store.recoverInterrupted(ctx, time.Now().UTC()); err != nil {
			return fmt.Errorf("recover interrupted scheduled deliveries: %w", err)
		}
		processed, err := w.ProcessNext(ctx)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		delay, err := w.Store.nextDelay(ctx)
		if err != nil {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-w.Store.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// ProcessNext atomically claims and processes one due job. Untyped transport
// failures are treated as ambiguous, never automatically retried.
func (w *Worker) ProcessNext(ctx context.Context) (bool, error) {
	if w.Store == nil || w.Transport == nil {
		return false, errors.New("scheduled worker requires a queue and transport")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	job, err := w.Store.claim(ctx, time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim scheduled delivery: %w", err)
	}
	command, clientKey, err := OpenCommand(job.envelope, w.Store.helper)
	if err != nil || command.Action != "schedule" || command.JobID != job.id || command.Revision != job.revision {
		return true, w.complete(ctx, job, Failed, "invalid_payload", time.Time{})
	}
	defer clear(command.Delivery.RFC822)
	permissions, err := w.Store.permissions(ctx, clientKey)
	if errors.Is(err, ErrUnpaired) || (err == nil && !permissions.allows(command.Delivery)) {
		return true, w.complete(ctx, job, Failed, "client_authorization_revoked", time.Time{})
	}
	if err != nil || ctx.Err() != nil {
		return true, w.complete(ctx, job, Retry, "local_authorization_unavailable", time.Now().UTC().Add(time.Minute))
	}
	// Credential lookup, connection setup, authentication, and submission must
	// all fit this limit. Interrupted claims are recovered only after 15 minutes.
	deliveryContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	err = w.Transport.Deliver(deliveryContext, *command.Delivery)
	cancel()
	if err == nil {
		return true, w.complete(ctx, job, Sent, "", time.Time{})
	}
	var failure *DeliveryError
	if !errors.As(err, &failure) || failure == nil {
		return true, w.complete(ctx, job, Unknown, "delivery_outcome_unknown", time.Time{})
	}
	switch failure.Kind {
	case AcceptedWithoutSentCopy:
		return true, w.complete(ctx, job, Sent, "sent_copy_failed", time.Time{})
	case SafeRetry:
		if job.attempts >= 8 {
			return true, w.complete(ctx, job, Failed, "retry_limit_reached", time.Time{})
		}
		delay := min(time.Hour, time.Minute*time.Duration(1<<uint(job.attempts-1)))
		return true, w.complete(ctx, job, Retry, "temporary_transport_failure", time.Now().UTC().Add(delay))
	case PermanentFailure:
		return true, w.complete(ctx, job, Failed, "permanent_transport_failure", time.Time{})
	case AuthorizationRequired:
		return true, w.complete(ctx, job, Failed, "authorization_required", time.Time{})
	default:
		return true, w.complete(ctx, job, Unknown, "delivery_outcome_unknown", time.Time{})
	}
}

func (w *Worker) complete(ctx context.Context, job claimedJob, state State, code string, next time.Time) error {
	// Persist even if shutdown cancelled the send context. Failure here leaves
	// the claim in 'sending', which later becomes unknown rather than retrying.
	writeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return w.Store.finish(writeContext, job, state, code, next)
}
