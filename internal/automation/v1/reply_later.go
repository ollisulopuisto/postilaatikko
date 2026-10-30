// Package v1 contains transport-independent operations for authenticated
// automation clients. A local HTTP/MCP transport must authenticate the caller
// and supply an Authorizer before exposing these operations.
package v1

import (
	"context"
	"errors"
	"fmt"

	"github.com/hkdb/aerion/internal/replylater"
)

var (
	ErrCapabilityDenied = errors.New("automation capability denied")
	ErrUnavailable      = errors.New("Reply Later automation is unavailable")
)

// Authorizer checks a capability against the already authenticated caller.
// Implementations must not infer identity or grants from request data.
type Authorizer interface {
	RequireCapability(context.Context, string) error
}

// ReplyLaterOperations is implemented by the application layer so writes
// continue through its normal durable undo/event pipeline.
type ReplyLaterOperations interface {
	GetReplyLaterQueue() ([]replylater.Ref, error)
	ChangeReplyLaterWithUndo(accountID, conversationID string, inQueue bool) (bool, string, error)
}

// ChangeResult mirrors the safe, operation-scoped result returned by the app.
type ChangeResult struct {
	Changed     bool   `json:"changed"`
	OperationID string `json:"operationId"`
}

// ReplyLaterAdapter exposes the personal triage queue to an authorized caller.
// It contains no listener or token management; UX-020 binds it to the
// authenticated local transport.
type ReplyLaterAdapter struct {
	operations ReplyLaterOperations
	authorizer Authorizer
}

func NewReplyLaterAdapter(operations ReplyLaterOperations, authorizer Authorizer) *ReplyLaterAdapter {
	return &ReplyLaterAdapter{operations: operations, authorizer: authorizer}
}

func (a *ReplyLaterAdapter) List(ctx context.Context) ([]replylater.Ref, error) {
	if err := a.require(ctx, CapabilityMailRead); err != nil {
		return nil, err
	}
	return a.operations.GetReplyLaterQueue()
}

func (a *ReplyLaterAdapter) Add(ctx context.Context, ref replylater.Ref) (ChangeResult, error) {
	if err := a.require(ctx, CapabilityMailWrite); err != nil {
		return ChangeResult{}, err
	}
	changed, operationID, err := a.operations.ChangeReplyLaterWithUndo(ref.AccountID, ref.ConversationID, true)
	return ChangeResult{Changed: changed, OperationID: operationID}, err
}

func (a *ReplyLaterAdapter) Remove(ctx context.Context, ref replylater.Ref) (ChangeResult, error) {
	if err := a.require(ctx, CapabilityMailWrite); err != nil {
		return ChangeResult{}, err
	}
	changed, operationID, err := a.operations.ChangeReplyLaterWithUndo(ref.AccountID, ref.ConversationID, false)
	return ChangeResult{Changed: changed, OperationID: operationID}, err
}

func (a *ReplyLaterAdapter) require(ctx context.Context, capability Capability) error {
	if a == nil || a.operations == nil || a.authorizer == nil {
		return ErrUnavailable
	}
	if err := a.authorizer.RequireCapability(ctx, string(capability)); err != nil {
		return fmt.Errorf("%w: %s", ErrCapabilityDenied, capability)
	}
	return nil
}
