package app

import (
	"fmt"

	agentapi "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/replylater"
	"github.com/hkdb/aerion/internal/undo"
)

// newReplyLaterAutomationAPI builds the transport-independent API adapter for
// an already authenticated automation caller. The local API transport owns
// authentication and capability grants; this method never creates a listener.
func (a *App) newReplyLaterAutomationAPI(authorizer agentapi.Authorizer) *agentapi.ReplyLaterAdapter {
	return agentapi.NewReplyLaterAdapter(replyLaterAutomationOperations{app: a}, authorizer)
}

type replyLaterAutomationOperations struct{ app *App }

var _ agentapi.ReplyLaterOperations = replyLaterAutomationOperations{}

func (ops replyLaterAutomationOperations) GetReplyLaterQueue() ([]replylater.Ref, error) {
	return ops.app.GetReplyLaterQueue()
}

func (ops replyLaterAutomationOperations) ChangeReplyLaterWithUndo(accountID, conversationID string, inQueue bool) (bool, string, error) {
	result, err := ops.app.ChangeReplyLaterWithUndo(accountID, conversationID, inQueue)
	return result.Changed, result.OperationID, err
}

// GetReplyLaterQueue returns explicit triage references, including references
// whose conversation is not currently present in the local message index.
func (a *App) GetReplyLaterQueue() ([]replylater.Ref, error) {
	if a.mailboxService != nil {
		ctx := a.helperCommandContext()
		status := a.mailboxService.status(ctx)
		if status.Connected {
			return a.mailboxService.replyLaterQueue(ctx)
		}
		if status.ErrorCode != "service_unavailable" {
			return nil, fmt.Errorf("load Reply Later queue from mailbox service: %s", status.Message)
		}
	}
	if a.replyLaterStore == nil {
		return nil, fmt.Errorf("Reply Later queue is not ready")
	}
	return a.replyLaterStore.List()
}

// GetReplyLaterCount returns the number of explicit queued conversations.
func (a *App) GetReplyLaterCount() (int, error) {
	queue, err := a.GetReplyLaterQueue()
	return len(queue), err
}

// IsReplyLater reports explicit queue membership for an account conversation.
func (a *App) IsReplyLater(accountID, conversationID string) (bool, error) {
	queue, err := a.GetReplyLaterQueue()
	if err != nil {
		return false, err
	}
	for _, ref := range queue {
		if ref.AccountID == accountID && ref.ConversationID == conversationID {
			return true, nil
		}
	}
	return false, nil
}

// ReplyLaterChangeResult exposes a targeted undo token for one queue change.
type ReplyLaterChangeResult struct {
	Changed     bool   `json:"changed"`
	OperationID string `json:"operationId"`
}

// AddReplyLater adds a conversation to the personal triage queue.
func (a *App) AddReplyLater(accountID, conversationID string) (bool, error) {
	result, err := a.ChangeReplyLaterWithUndo(accountID, conversationID, true)
	return result.Changed, err
}

// RemoveReplyLater removes a conversation without changing provider flags.
func (a *App) RemoveReplyLater(accountID, conversationID string) (bool, error) {
	result, err := a.ChangeReplyLaterWithUndo(accountID, conversationID, false)
	return result.Changed, err
}

// ChangeReplyLaterWithUndo sets explicit membership and returns the normal
// operation-scoped undo token. Duplicate calls do not create duplicate history.
func (a *App) ChangeReplyLaterWithUndo(accountID, conversationID string, inQueue bool) (ReplyLaterChangeResult, error) {
	ref := replylater.Ref{AccountID: accountID, ConversationID: conversationID}
	changed, operationID, handled, err := a.changeReplyLaterThroughService(ref, inQueue)
	if handled {
		if err != nil {
			return ReplyLaterChangeResult{}, err
		}
		if changed {
			a.emitReplyLaterChanged(ref, inQueue)
			a.emitRuntimeEvent("undo:history-changed")
		}
		return ReplyLaterChangeResult{Changed: changed, OperationID: operationID}, nil
	}
	if a.replyLaterStore == nil {
		return ReplyLaterChangeResult{}, fmt.Errorf("Reply Later queue is not ready")
	}
	if err := a.changeReplyLaterProvider(accountID, conversationID, inQueue); err != nil {
		return ReplyLaterChangeResult{}, err
	}
	if inQueue {
		changed, err = a.replyLaterStore.Add(ref)
	} else {
		changed, err = a.replyLaterStore.Remove(ref)
	}
	if err != nil || !changed {
		return ReplyLaterChangeResult{Changed: changed}, err
	}
	description, action := "Added to Reply Later", "reply-later-add"
	if !inQueue {
		description, action = "Removed from Reply Later", "reply-later-remove"
	}
	operationID = a.undoStack.PushOperation("", undo.NewReplyLaterChangeCommand(a, ref, !inQueue, description), undo.OperationMetadata{Action: action})
	a.emitReplyLaterChanged(ref, inQueue)
	a.emitRuntimeEvent("undo:history-changed")
	return ReplyLaterChangeResult{Changed: true, OperationID: operationID}, nil
}

// SetReplyLaterMembership is the undo-context operation for restoring a
// previous explicit membership state.
func (a *App) SetReplyLaterMembership(ref replylater.Ref, inQueue bool) error {
	changed, _, handled, err := a.changeReplyLaterThroughService(ref, inQueue)
	if handled {
		if err != nil {
			return err
		}
		if changed {
			a.emitReplyLaterChanged(ref, inQueue)
		}
		return nil
	}
	if a.replyLaterStore == nil {
		return fmt.Errorf("Reply Later queue is not ready")
	}
	if err := a.changeReplyLaterProvider(ref.AccountID, ref.ConversationID, inQueue); err != nil {
		return err
	}
	if inQueue {
		changed, err = a.replyLaterStore.Add(ref)
	} else {
		changed, err = a.replyLaterStore.Remove(ref)
	}
	if err != nil {
		return err
	}
	if changed {
		a.emitReplyLaterChanged(ref, inQueue)
	}
	return nil
}

func (a *App) changeReplyLaterThroughService(ref replylater.Ref, inQueue bool) (bool, string, bool, error) {
	if a.mailboxService == nil {
		return false, "", false, nil
	}
	ctx := a.helperCommandContext()
	status := a.mailboxService.status(ctx)
	if !status.Connected {
		if status.ErrorCode == "service_unavailable" {
			return false, "", false, nil
		}
		return false, "", true, fmt.Errorf("mailbox service is not ready for Reply Later: %s", status.Message)
	}
	if !status.WriteAuthorized {
		return false, "", true, errDesktopMailboxWriteAuthorizationRequired
	}
	response, err := a.mailboxService.setReplyLaterMembershipWithReceipt(ctx, ref, inQueue)
	if err != nil {
		return false, "", true, fmt.Errorf("update Reply Later through local mailbox service: %w", err)
	}
	return response.Changed, response.Receipt.OperationID, true, nil
}

func (a *App) emitReplyLaterChanged(ref replylater.Ref, inQueue bool) {
	a.emitRuntimeEvent("replyLater:changed", map[string]interface{}{
		"accountId":      ref.AccountID,
		"conversationId": ref.ConversationID,
		"inQueue":        inQueue,
	})
}
