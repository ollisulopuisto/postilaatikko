package scheduledsend

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  FailureKind
		state State
		code  string
	}{
		{"safe retry", SafeRetry, Retry, "temporary_transport_failure"},
		{"permanent rejection", PermanentFailure, Failed, "permanent_transport_failure"},
		{"expired credentials", AuthorizationRequired, Failed, "authorization_required"},
		{"unknown acceptance", OutcomeUnknown, Unknown, "delivery_outcome_unknown"},
		{"sent copy failure", AcceptedWithoutSentCopy, Sent, "sent_copy_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, client, helper := baselineQueue(t)
			command := baselineCommand()
			envelope := baselineSeal(t, command, client, helper)
			if _, err := store.Accept(ctx, envelope); err != nil {
				t.Fatal(err)
			}
			failure := &DeliveryError{Kind: tc.kind, Err: errors.New("secret provider recipient/token diagnostic")}
			calls := 0
			worker := Worker{Store: store, Transport: baselineTransport(func(context.Context, Delivery) error { calls++; return failure })}
			if processed, err := worker.ProcessNext(ctx); err != nil || !processed {
				t.Fatalf("process: %v %v", processed, err)
			}
			status, err := store.GetStatus(ctx, command.JobID)
			if err != nil {
				t.Fatal(err)
			}
			if status.State != tc.state || status.Code != tc.code || status.Attempts != 1 {
				t.Fatalf("status: %+v", status)
			}
			if strings.Contains(failure.Error(), "secret") {
				t.Fatal("provider diagnostic exposed")
			}
			// Replaying accepted bytes must report progress, never reset attempts.
			if replay, err := store.Accept(ctx, envelope); err != nil || replay.State != tc.state || replay.Attempts != 1 {
				t.Fatalf("replay: %+v %v", replay, err)
			}
			if processed, err := worker.ProcessNext(ctx); err != nil || processed {
				t.Fatalf("automatic resend: %v %v", processed, err)
			}
			if calls != 1 {
				t.Fatalf("delivery calls: %d", calls)
			}
			var payload []byte
			if err := store.db.QueryRow("SELECT envelope FROM jobs WHERE id = ?", command.JobID).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) == 0 {
				t.Fatal("recoverable encrypted payload removed")
			}
			if tc.kind == SafeRetry {
				if _, err := store.db.Exec("UPDATE jobs SET attempts = 7, next_attempt = 0 WHERE id = ?", command.JobID); err != nil {
					t.Fatal(err)
				}
				if processed, err := worker.ProcessNext(ctx); err != nil || !processed {
					t.Fatalf("last retry: %v %v", processed, err)
				}
				status, err := store.GetStatus(ctx, command.JobID)
				if err != nil || status.State != Failed || status.Attempts != 8 || status.Code != "retry_limit_reached" {
					t.Fatalf("limit: %+v %v", status, err)
				}
			}
		})
	}
}

func TestRecoveryInterruptedClaimCannotBeResentOrOverwritten(t *testing.T) {
	for _, outcome := range []State{Sent, Failed} {
		t.Run(string(outcome), func(t *testing.T) {
			ctx := context.Background()
			store, client, helper := baselineQueue(t)
			command := baselineCommand()
			envelope := baselineSeal(t, command, client, helper)
			if _, err := store.Accept(ctx, envelope); err != nil {
				t.Fatal(err)
			}
			job, err := store.claim(ctx, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			var claimedAt int64
			if err := store.db.QueryRow("SELECT updated_at FROM jobs WHERE id = ?", job.id).Scan(&claimedAt); err != nil {
				t.Fatal(err)
			}
			// Another helper connection represents restart/recovery arbitration.
			other := reopenHelperQueue(t, store)
			claimedTime := time.UnixMilli(claimedAt)
			if err := other.recoverInterrupted(ctx, claimedTime.Add(14*time.Minute)); err != nil {
				t.Fatal(err)
			}
			status, err := other.GetStatus(ctx, job.id)
			if err != nil || status.State != Sending {
				t.Fatalf("active claim recovered early: %+v %v", status, err)
			}
			if err := other.recoverInterrupted(ctx, claimedTime.Add(16*time.Minute)); err != nil {
				t.Fatal(err)
			}
			status, err = other.GetStatus(ctx, job.id)
			if err != nil || status.State != Unknown || status.Code != "interrupted_delivery" {
				t.Fatalf("recovery: %+v %v", status, err)
			}
			if err := store.finish(ctx, job, Sent, "", time.Time{}); !errors.Is(err, ErrStateChanged) {
				t.Fatalf("stale worker overwrote recovery: %v", err)
			}
			worker := Worker{Store: other, Transport: baselineTransport(func(context.Context, Delivery) error { t.Error("ambiguous mail resent"); return nil })}
			if processed, err := worker.ProcessNext(ctx); err != nil || processed {
				t.Fatalf("recovered work: %v %v", processed, err)
			}
			command.Revision++
			if _, err := other.Accept(ctx, baselineSeal(t, command, client, helper)); !errors.Is(err, ErrDeliveryStarted) {
				t.Fatalf("unknown replacement: %v", err)
			}
			if err := other.ResolveUnknown(ctx, job.id, job.revision+1, outcome); !errors.Is(err, ErrStateChanged) {
				t.Fatalf("wrong revision resolution: %v", err)
			}
			if err := other.ResolveUnknown(ctx, job.id, job.revision, Retry); err == nil {
				t.Fatal("automatic retry resolution accepted")
			}
			if err := other.ResolveUnknown(ctx, job.id, job.revision, outcome); err != nil {
				t.Fatal(err)
			}
			if err := store.finish(ctx, job, Sent, "", time.Time{}); !errors.Is(err, ErrStateChanged) {
				t.Fatalf("late worker overwrote manual resolution: %v", err)
			}
			if replay, err := other.Accept(ctx, envelope); err != nil || replay.State != outcome {
				t.Fatalf("resolved replay: %+v %v", replay, err)
			}
			var payload []byte
			if err := other.db.QueryRow("SELECT envelope FROM jobs WHERE id = ?", job.id).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if (len(payload) == 0) != (outcome == Sent) {
				t.Fatalf("payload retention after %s: %d", outcome, len(payload))
			}
		})
	}
}

func reopenHelperQueue(t *testing.T, store *Store) *Store {
	t.Helper()
	var index int
	var name, path string
	if err := store.db.QueryRow("PRAGMA database_list").Scan(&index, &name, &path); err != nil {
		t.Fatal(err)
	}
	other, err := OpenStore(filepath.Dir(path), store.helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	})
	return other
}

func TestRecoveryConcurrentClaimAndCancellation(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	other := reopenHelperQueue(t, store)
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	worker := Worker{Store: store, Transport: baselineTransport(func(context.Context, Delivery) error { close(started); <-release; return nil })}
	done := make(chan error, 1)
	go func() { _, err := worker.ProcessNext(ctx); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	// Cancellation and rescheduling must be rejected after the atomic claim.
	for _, action := range []string{"cancel", "schedule"} {
		change := command
		change.Action, change.Revision = action, 2
		if action == "cancel" {
			change.Delivery = nil
		}
		if _, err := other.Accept(ctx, baselineSeal(t, change, client, helper)); !errors.Is(err, ErrDeliveryStarted) {
			t.Fatalf("%s in flight: %v", action, err)
		}
	}
	second := Worker{Store: other, Transport: baselineTransport(func(context.Context, Delivery) error { t.Error("second helper delivered claim"); return nil })}
	if processed, err := second.ProcessNext(ctx); err != nil || processed {
		t.Fatalf("second claim: %v %v", processed, err)
	}
	// Closing via a separate channel permits waiting before the deferred release.
	go func() { release <- struct{}{} }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish")
	}
	status, err := other.GetStatus(ctx, command.JobID)
	if err != nil || status.State != Sent || status.Attempts != 1 {
		t.Fatalf("completed claim: %+v %v", status, err)
	}
}

func TestRecoveryAcceptedSendWithFailedCompletionWrite(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	// Inject a real persistence failure after provider acceptance, without
	// breaking the earlier atomic claim or relying on timing/disk capacity.
	if _, err := store.db.Exec(`CREATE TRIGGER fail_completion BEFORE UPDATE OF state ON jobs
		WHEN NEW.state = 'sent' BEGIN SELECT RAISE(FAIL, 'injected completion failure'); END`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	worker := Worker{Store: store, Transport: baselineTransport(func(context.Context, Delivery) error { calls++; return nil })}
	if processed, err := worker.ProcessNext(ctx); err == nil || !processed {
		t.Fatalf("completion failure not surfaced: %v %v", processed, err)
	}
	status, err := store.GetStatus(ctx, command.JobID)
	if err != nil || status.State != Sending {
		t.Fatalf("failed completion lost claim: %+v %v", status, err)
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_completion"); err != nil {
		t.Fatal(err)
	}
	other := reopenHelperQueue(t, store)
	if err := other.recoverInterrupted(ctx, time.Now().Add(16*time.Minute)); err != nil {
		t.Fatal(err)
	}
	worker.Store = other
	if processed, err := worker.ProcessNext(ctx); err != nil || processed {
		t.Fatalf("accepted mail retried after storage failure: %v %v", processed, err)
	}
	status, err = other.GetStatus(ctx, command.JobID)
	if err != nil || status.State != Unknown || status.Code != "interrupted_delivery" || calls != 1 {
		t.Fatalf("completion recovery: %+v %v, delivery calls %d", status, err, calls)
	}
}
