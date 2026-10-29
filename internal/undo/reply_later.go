package undo

import "github.com/hkdb/aerion/internal/replylater"

// ReplyLaterContext is the small app capability needed to reverse queue
// membership without giving undo commands direct access to Wails runtime APIs.
type ReplyLaterContext interface {
	SetReplyLaterMembership(ref replylater.Ref, inQueue bool) error
}

// ReplyLaterChangeCommand reverses one explicit triage queue membership
// change. Provider replied flags are intentionally unrelated.
type ReplyLaterChangeCommand struct {
	BaseCommand
	ctx     ReplyLaterContext
	ref     replylater.Ref
	inQueue bool
}

func NewReplyLaterChangeCommand(ctx ReplyLaterContext, ref replylater.Ref, previousState bool, description string) *ReplyLaterChangeCommand {
	return &ReplyLaterChangeCommand{
		BaseCommand: NewBaseCommand(description),
		ctx:         ctx,
		ref:         ref,
		inQueue:     previousState,
	}
}

func (c *ReplyLaterChangeCommand) Execute() error { return nil }

func (c *ReplyLaterChangeCommand) Undo() error {
	return c.ctx.SetReplyLaterMembership(c.ref, c.inQueue)
}
