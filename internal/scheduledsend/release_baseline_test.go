package scheduledsend

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

type baselineTransport func(context.Context, Delivery) error

func (f baselineTransport) Deliver(ctx context.Context, d Delivery) error { return f(ctx, d) }

func TestReleaseBaselineDeliveryLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		state   State
		code    string
	}{
		{name: "accepted", state: Sent},
		{name: "ambiguous submission", failure: errors.New("connection lost"), state: Unknown, code: "delivery_outcome_unknown"},
		{name: "accepted without sent copy", failure: &DeliveryError{Kind: AcceptedWithoutSentCopy}, state: Sent, code: "sent_copy_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, client, helper := baselineQueue(t)
			command := baselineCommand()
			envelope := baselineSeal(t, command, client, helper)
			if _, err := store.Accept(ctx, envelope); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Accept(ctx, envelope); err != nil {
				t.Fatalf("idempotent replay: %v", err)
			}
			calls := 0
			worker := Worker{Store: store, Transport: baselineTransport(func(_ context.Context, d Delivery) error {
				calls++
				if d.From != command.Delivery.From || string(d.RFC822) != string(command.Delivery.RFC822) {
					t.Error("delivery changed in transit")
				}
				return tc.failure
			})}
			if processed, err := worker.ProcessNext(ctx); err != nil || !processed {
				t.Fatalf("process: %v, %v", processed, err)
			}
			status, err := store.GetStatus(ctx, command.JobID)
			if err != nil {
				t.Fatal(err)
			}
			if status.State != tc.state || status.Code != tc.code {
				t.Fatalf("status: %+v", status)
			}
			if processed, err := worker.ProcessNext(ctx); err != nil || processed {
				t.Fatalf("unexpected repeat: %v, %v", processed, err)
			}
			if calls != 1 {
				t.Fatalf("delivery calls: %d", calls)
			}
		})
	}
}

func TestReleaseBaselineCancellationAndAuthorization(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	envelope := baselineSeal(t, command, client, helper)
	if _, err := store.Accept(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	cancel := Command{Version: ProtocolVersion, JobID: command.JobID, Revision: 2, Action: "cancel"}
	if status, err := store.Accept(ctx, baselineSeal(t, cancel, client, helper)); err != nil || status.State != Cancelled {
		t.Fatalf("cancel: %+v, %v", status, err)
	}
	if _, err := store.Accept(ctx, envelope); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("old replay accepted: %v", err)
	}
	worker := Worker{Store: store, Transport: baselineTransport(func(context.Context, Delivery) error { t.Fatal("cancelled or revoked mail delivered"); return nil })}
	if processed, err := worker.ProcessNext(ctx); err != nil || processed {
		t.Fatalf("cancelled work: %v, %v", processed, err)
	}
	command.JobID = uuid.NewString()
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeClient(ctx, client.PublicKey()); err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.ProcessNext(ctx); err != nil || processed {
		t.Fatalf("revoked work: %v, %v", processed, err)
	}
	command.JobID = uuid.NewString()
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); !errors.Is(err, ErrUnpaired) {
		t.Fatalf("revoked submission: %v", err)
	}
}

func TestReleaseBaselineRejectsTamperingAndOtherAccount(t *testing.T) {
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	envelope := baselineSeal(t, command, client, helper)
	// Change a valid JSON envelope without modifying its syntax.
	var decoded Envelope
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Ciphertext[0] ^= 1
	tampered, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Accept(context.Background(), tampered); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	command.Delivery.AccountID = "unauthorized-account"
	if _, err := store.Accept(context.Background(), baselineSeal(t, command, client, helper)); err == nil {
		t.Fatal("unauthorized account accepted")
	}
}

func baselineQueue(t *testing.T) (*Store, *Identity, *Identity) {
	t.Helper()
	helper, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	client, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(t.TempDir(), "queue"), helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := store.PairClient(context.Background(), client.PublicKey(), Permissions{Senders: []SenderPermission{{AccountID: "test-account", From: "sender@example.com"}}}); err != nil {
		t.Fatal(err)
	}
	return store, client, helper
}

func baselineCommand() Command {
	return Command{Version: ProtocolVersion, JobID: uuid.NewString(), Revision: 1, Action: "schedule", Delivery: &Delivery{
		AccountID: "test-account", From: "sender@example.com", To: []string{"recipient@example.com"}, DueAt: time.Now().Add(-time.Minute),
		RFC822: []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nMessage-ID: <baseline@example.com>\r\nSubject: Queue regression\r\n\r\nPrivate message body\r\n"),
	}}
}

func baselineSeal(t *testing.T, command Command, client, helper *Identity) []byte {
	t.Helper()
	encoded, err := SealCommand(command, client, helper.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
