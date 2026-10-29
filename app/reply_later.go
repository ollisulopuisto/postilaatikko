package app

import (
	"fmt"

	"github.com/hkdb/aerion/internal/replylater"
	"github.com/hkdb/aerion/internal/undo"
)

// GetReplyLaterQueue returns explicit triage references, including references
// whose conversation is not currently present in the local message index.
func (a *App) GetReplyLaterQueue() ([]replylater.Ref, error) {
	if a.replyLaterStore == nil {
		return nil, fmt.Errorf("Reply Later queue is not ready")
	}
	return a.replyLaterStore.List()
}

func (a *App) GetReplyLaterCount() (int, error) {
	if a.replyLaterStore == nil {
		return 0, fmt.Errorf("Reply Later queue is not ready")
	}
	return a.replyLaterStore.Count()
}

// IsReplyLater reports explicit queue membership for an account conversation.
func (a *App) IsReplyLater(accountID, conversationID string) (bool, error) {
	if a.replyLaterStore == nil {
		return false, fmt.Errorf("Reply Later queue is not ready")
	}
	return a.replyLaterStore.Contains(replylater.Ref{AccountID: accountID, ConversationID: conversationID})
}

// AddReplyLater adds a conversation to the personal triage queue. Repeated
// adds are harmless and do not create redundant undo entries.
func (a *App) AddReplyLater(accountID, conversationID string) (bool, error) {
	if a.replyLaterStore == nil {
		return false, fmt.Errorf("Reply Later queue is not ready")
	}
	ref := replylater.Ref{AccountID: accountID, ConversationID: conversationID}
	changed, err := a.replyLaterStore.Add(ref)
	if err != nil || !changed {
		return changed, err
	}
	a.emitReplyLaterChanged(ref, true)
	a.undoStack.PushOperation("", undo.NewReplyLaterChangeCommand(a, ref, false, "Added to Reply Later"), undo.OperationMetadata{Action: "reply-later-add"})
	return true, nil
}

// RemoveReplyLater removes a conversation from the triage queue. It leaves
// message flags and provider state untouched.
func (a *App) RemoveReplyLater(accountID, conversationID string) (bool, error) {
	if a.replyLaterStore == nil {
		return false, fmt.Errorf("Reply Later queue is not ready")
	}
	ref := replylater.Ref{AccountID: accountID, ConversationID: conversationID}
	changed, err := a.replyLaterStore.Remove(ref)
	if err != nil || !changed {
		return changed, err
	}
	a.emitReplyLaterChanged(ref, false)
	a.undoStack.PushOperation("", undo.NewReplyLaterChangeCommand(a, ref, true, "Removed from Reply Later"), undo.OperationMetadata{Action: "reply-later-remove"})
	return true, nil
}

// SetReplyLaterMembership is the undo-context operation for restoring a
// previous explicit membership state.
func (a *App) SetReplyLaterMembership(ref replylater.Ref, inQueue bool) error {
	if a.replyLaterStore == nil {
		return fmt.Errorf("Reply Later queue is not ready")
	}
	var changed bool
	var err error
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

func (a *App) emitReplyLaterChanged(ref replylater.Ref, inQueue bool) {
	a.emitRuntimeEvent("replyLater:changed", map[string]interface{}{
		"accountId":      ref.AccountID,
		"conversationId": ref.ConversationID,
		"inQueue":        inQueue,
	})
}
