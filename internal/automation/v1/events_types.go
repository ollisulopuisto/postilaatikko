package v1

import "time"

// Mailbox change event types (PERF-SC-004). Events carry invalidation
// metadata only: never message contents, credentials, or filesystem paths.
const (
	// EventMailboxChanged reports new, changed, or removed messages in one
	// account folder (provider sync, mail actions, undo, filters).
	EventMailboxChanged = "mailbox.changed"
	// EventFoldersChanged reports a change to an account's folder catalog.
	EventFoldersChanged = "folders.changed"
	// EventViewsChanged reports saved-view creation, mutation, or
	// invalidation.
	EventViewsChanged = "views.changed"
	// EventDraftsChanged reports a draft create, update, or discard.
	EventDraftsChanged = "drafts.changed"
	// EventTriageChanged reports a triage or reply-later state change.
	EventTriageChanged = "triage.changed"
	// EventScheduledSendChanged reports scheduled-send state changes.
	EventScheduledSendChanged = "scheduledsend.changed"
	// EventResync tells the subscriber its event stream has a gap it cannot
	// repair from the server's buffer (or its buffered backlog overflowed)
	// and it must re-read mailbox state, e.g. via /v1/workspace/bootstrap.
	EventResync = "mailbox.resync"
)

// MailboxEvent is one change notification on the mailbox event stream.
// Sequence is strictly monotonic per helper process lifetime; a subscriber
// detects gaps by comparing each Sequence to the previous one plus one.
type MailboxEvent struct {
	Sequence  uint64    `json:"sequence"`
	Type      string    `json:"type"`
	AccountID string    `json:"accountId,omitempty"`
	FolderID  string    `json:"folderId,omitempty"`
	At        time.Time `json:"at"`
}

// MailboxEventSource feeds the /v1/events stream. Implemented by the helper's
// event broker; defined here so the transport does not depend on the broker
// package.
type MailboxEventSource interface {
	// Subscribe registers a subscriber. replay holds buffered events with
	// Sequence > afterSeq that were still retained; resync reports that
	// events after afterSeq were already dropped and the subscriber must
	// reconcile fully. The channel delivers live events after the replay and
	// is closed when the source shuts down. cancel must be called when the
	// subscriber stops reading; it is idempotent.
	Subscribe(afterSeq uint64) (replay []MailboxEvent, ch <-chan MailboxEvent, resync bool, cancel func(), err error)
}

// GrantRevocationChecker lets the long-lived events stream re-check that the
// connecting grant has not been revoked since authentication.
type GrantRevocationChecker interface {
	// GrantRevoked reports whether the grant with this client ID is revoked
	// or gone.
	GrantRevoked(clientID string) (bool, error)
}
