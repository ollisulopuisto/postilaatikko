package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/pendingremote"
	"github.com/hkdb/aerion/internal/smtp"
)

type remotePendingEnvelope struct {
	Endpoint string         `json:"endpoint"`
	Request  v1.SendRequest `json:"request"`
}

func (a *App) remotePendingStore() (*pendingremote.Store, error) {
	if a.deviceDB == nil {
		return nil, errors.New("device database is unavailable")
	}
	return pendingremote.New(a.deviceDB.DB)
}
func (a *App) queueRemotePendingSend(accountID, draftID, payload string) (string, error) {
	if accountID == "" {
		return "", errors.New("pending send account is required")
	}
	if len(payload) > pendingremote.MaxPayloadBytes {
		return "", errors.New("pending-send payload is too large")
	}
	var msg smtp.ComposeMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return "", errors.New("pending-send payload is invalid")
	}
	required := []v1.Capability{v1.CapabilityMailSend}
	if draftID != "" {
		required = append(required, v1.CapabilityDraftRead, v1.CapabilityDraftWrite)
	}
	client, err := a.composerServiceClient(a.accountContext(), required...)
	if err != nil {
		return "", err
	}
	revision := ""
	if draftID != "" {
		doc, err := client.GetDraft(a.accountContext(), accountID, draftID)
		if err != nil {
			return "", err
		}
		if doc == nil || doc.AccountID != accountID {
			return "", errors.New("pending draft does not belong to this account")
		}
		revision = doc.Revision
	}
	request, err := a.PrepareComposerSend(accountID, msg, draftID, revision)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(remotePendingEnvelope{Endpoint: a.remoteServiceEndpoint, Request: *request})
	if err != nil {
		return "", err
	}
	defer clear(encoded)
	defer clear(request.RFC822)
	store, err := a.remotePendingStore()
	if err != nil {
		return "", err
	}
	a.reconcileRemotePendingOutcomes(store)
	now := time.Now().UTC()
	id := uuid.NewString()
	if err := store.Queue(pendingremote.Item{ID: id, AccountID: accountID, DraftID: draftID, Payload: payload, Prepared: encoded, DispatchAt: now.Add(PendingSendDelay), CreatedAt: now}); err != nil {
		return "", err
	}
	a.holdPendingSendActivity(id)
	a.emitRuntimeEvent("pending-send:queued", PendingSend{ID: id, AccountID: accountID, DraftID: draftID, Payload: payload, DispatchAt: now.Add(PendingSendDelay).Format(time.RFC3339Nano), CreatedAt: now.Format(time.RFC3339Nano), State: "pending"})
	return id, nil
}

// DispatchRemotePendingSend atomically closes the remote Undo Send cancellation
// window and submits immutable MIME with the queue ID as its durable request ID.
// False means local mode, whose existing dispatcher remains unchanged.
func (a *App) DispatchRemotePendingSend(id string) (bool, error) {
	if !a.remoteServiceMode {
		return false, nil
	}
	store, err := a.remotePendingStore()
	if err != nil {
		return true, err
	}
	item, err := store.Claim(id)
	if err != nil {
		return true, err
	}
	defer a.releasePendingSendActivity(id)
	defer clear(item.Prepared)
	finish := func(state string, original error) (bool, error) {
		if err := store.Finish(id, state); err != nil {
			return true, fmt.Errorf("send outcome could not be stored; do not resend: %w", err)
		}
		return true, original
	}
	var envelope remotePendingEnvelope
	if err := json.Unmarshal(item.Prepared, &envelope); err != nil {
		return finish(pendingremote.StateFailed, errors.New("stored send request is invalid"))
	}
	defer clear(envelope.Request.RFC822)
	if a.paths == nil {
		return finish(pendingremote.StateFailed, errors.New("device paths unavailable"))
	}
	if envelope.Endpoint != a.remoteServiceEndpoint || envelope.Request.AccountID != item.AccountID || envelope.Request.DraftID != item.DraftID {
		return finish(pendingremote.StateFailed, errors.New("mailbox connection changed; review the draft before sending"))
	}
	// This preserves arbitration with this device's existing Send Later client.
	// It never opens the local mailbox database or constructs a provider client.
	if err := guardScheduledDraft(a.accountContext(), a.paths.Data, item.AccountID, item.DraftID); err != nil {
		return finish(pendingremote.StateFailed, err)
	}
	response, err := a.SendPreparedComposerMessage(envelope.Request, id)
	if err != nil {
		// Only these structured helper codes are definitive pre-delivery rejections.
		// Internal/provider/idempotency errors may follow reservation or acceptance.
		for _, code := range []v1.ErrorCode{v1.CodeRevisionConflict, v1.CodeInvalidRequest, v1.CodeNotFound, v1.CodeCapabilityDenied, v1.CodeUnauthenticated} {
			if v1.IsCode(err, code) {
				return finish(pendingremote.StateFailed, fmt.Errorf("helper rejected delivery; draft retained: %w", err))
			}
		}
		// An HTTP failure does not establish that the provider rejected delivery.
		lookup, lookupErr := a.GetComposerSendReceipt(id)
		if lookupErr == nil && lookup != nil && lookup.Found && lookup.Response != nil {
			response = lookup.Response
		} else {
			return finish(pendingremote.StateUnknown, errors.New("delivery outcome is unknown; do not send this draft again until the helper receipt is checked"))
		}
	}
	if response == nil {
		return finish(pendingremote.StateUnknown, errors.New("helper returned no delivery receipt; do not resend"))
	}
	switch response.Receipt.State {
	case v1.OperationSent:
		if response.Code != "" {
			a.emitRuntimeEvent("pending-send:warning", "Message sent; helper reported "+response.Code)
		}
		return finish(pendingremote.StateSent, nil)
	case v1.OperationUnknown, v1.OperationSending:
		return finish(pendingremote.StateUnknown, errors.New("helper delivery outcome is unknown; do not resend this draft"))
	default:
		return finish(pendingremote.StateFailed, fmt.Errorf("helper did not accept delivery (%s); draft retained", response.Receipt.State))
	}
}

// reconcileRemotePendingOutcomes queries receipts without submitting any message.
// Missing or in-flight receipts remain unknown and keep their draft protected.
func (a *App) reconcileRemotePendingOutcomes(store *pendingremote.Store) {
	ids, err := store.Uncertain()
	if err != nil || len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(a.accountContext(), 3*time.Second)
	defer cancel()
	client, err := a.composerServiceClient(ctx, v1.CapabilityMailSend)
	if err != nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		item, err := store.Connection(id)
		if err != nil {
			continue
		}
		var envelope remotePendingEnvelope
		err = json.Unmarshal(item.Prepared, &envelope)
		clear(item.Prepared)
		if err != nil || envelope.Endpoint != a.remoteServiceEndpoint {
			clear(envelope.Request.RFC822)
			continue
		}
		clear(envelope.Request.RFC822)
		lookup, err := client.GetSendReceipt(ctx, id)
		if err != nil || lookup == nil || !lookup.Found || lookup.Response == nil {
			continue
		}
		switch lookup.Response.Receipt.State {
		case v1.OperationSent:
			if err := store.Finish(id, pendingremote.StateSent); err == nil {
				a.emitRuntimeEvent("pending-send:warning", "A previously uncertain message was confirmed sent by the helper.")
			}
		case v1.OperationFailed, v1.OperationRetry:
			_ = store.Finish(id, pendingremote.StateFailed)
		}
	}
}
