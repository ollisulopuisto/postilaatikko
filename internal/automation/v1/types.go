// Package v1 defines the transport-independent wire contract for external
// automation clients. The local HTTP transport is implemented separately.
package v1

import (
	"fmt"

	"github.com/hkdb/aerion/internal/email/replycontent"
	"github.com/hkdb/aerion/internal/views"
)

// Version is the versioned route prefix and contract version.
const Version = "v1"

// Capability grants one independently checked kind of access to a client.
type Capability string

const (
	// CapabilityMailRead allows cached account, folder, search, thread and body reads.
	CapabilityMailRead Capability = "mail.read"
	// CapabilityMailBodyFetch allows an explicit provider body fetch.
	CapabilityMailBodyFetch Capability = "mail.body.fetch"
	// CapabilityAttachmentRead allows attachment discovery and download.
	CapabilityAttachmentRead Capability = "mail.attachment.read"
	// CapabilityDraftRead allows listing and reading drafts.
	CapabilityDraftRead Capability = "draft.read"
	// CapabilityDraftWrite allows creating, updating and discarding drafts.
	CapabilityDraftWrite Capability = "draft.write"
	// CapabilityMailWrite allows mailbox state changes and message moves.
	CapabilityMailWrite Capability = "mail.write"
	// CapabilityMailSend allows immediate delivery of a draft.
	CapabilityMailSend Capability = "mail.send"
	// CapabilityScheduleRead allows reading scheduled delivery state.
	CapabilityScheduleRead Capability = "schedule.read"
	// CapabilityScheduleWrite allows scheduling, cancelling and rescheduling delivery.
	CapabilityScheduleWrite Capability = "schedule.write"
)

// Known reports whether c is a capability defined by this API version.
func (c Capability) Known() bool {
	switch c {
	case CapabilityMailRead, CapabilityMailBodyFetch, CapabilityAttachmentRead,
		CapabilityDraftRead, CapabilityDraftWrite, CapabilityMailWrite,
		CapabilityMailSend, CapabilityScheduleRead, CapabilityScheduleWrite:
		return true
	default:
		return false
	}
}

// Capabilities describes the contract version and grants for the authenticated
// client. It never includes the client's credential.
type Capabilities struct {
	Version      string       `json:"version"`
	Capabilities []Capability `json:"capabilities"`
}

// Scope is the shared UX-002 view scope. Query semantics are defined by UX-003.
type Scope = views.Scope

// MessageRef identifies one message copy in one account.
type MessageRef = views.MessageRef

// ThreadRef identifies one thread in one account.
type ThreadRef = views.ThreadRef

// Completeness reports whether message bodies are present in a response.
type Completeness = views.Completeness

// DefaultPageSize is used when a collection request omits its limit.
const DefaultPageSize = 50

// MaximumPageSize is the hard maximum for generic collection requests.
const MaximumPageSize = 100

// PageRequest is a bounded collection request. Cursor is opaque to clients.
type PageRequest struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// Normalized returns the effective page size, rejecting values outside the
// public contract. Endpoint-specific limits may be lower than MaximumPageSize.
func (p PageRequest) Normalized() (PageRequest, error) {
	if p.Limit == 0 {
		p.Limit = DefaultPageSize
	}
	if p.Limit < 1 || p.Limit > MaximumPageSize {
		return PageRequest{}, fmt.Errorf("limit must be between 1 and %d", MaximumPageSize)
	}
	return p, nil
}

// Page is a bounded list response. Complete is false when any account failed
// or the server could not finish the requested scan.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	Complete   bool   `json:"complete"`
}

// SearchRequest asks for one bounded page of metadata matching a shared view
// scope. Search scopes carry the query expression.
type SearchRequest struct {
	Scope Scope       `json:"scope"`
	Page  PageRequest `json:"page,omitempty"`
}

// ThreadReadRequest reads a thread and marks which messages match the scope.
type ThreadReadRequest struct {
	Ref   ThreadRef `json:"ref"`
	Scope Scope     `json:"scope"`
}

// BodyReadRequest reads a cached body or explicitly requests a provider fetch.
type BodyReadRequest struct {
	Ref             MessageRef `json:"ref"`
	Mode            FetchMode  `json:"mode,omitempty"`
	IncludeOriginal bool       `json:"includeOriginal,omitempty"`
}

// MessageBody contains derived reply text and, only when requested, the
// sanitized original body. Derived text never replaces the original.
type MessageBody struct {
	Ref          MessageRef           `json:"ref"`
	ContentState views.ContentState   `json:"contentState"`
	Extracted    *replycontent.Result `json:"extracted,omitempty"`
	Original     *OriginalBody        `json:"original,omitempty"`
}

// OriginalBody contains locally sanitized display content, not active email
// markup. Remote images and executable markup are never included or loaded.
type OriginalBody struct {
	Text string `json:"text,omitempty"`
	HTML string `json:"html,omitempty"`
}

// AttachmentListRequest lists a bounded set of attachment metadata for a
// specific account-qualified message copy.
type AttachmentListRequest struct {
	Ref  MessageRef  `json:"ref"`
	Page PageRequest `json:"page,omitempty"`
}

// AttachmentReadRequest explicitly reads one attachment from a message copy.
type AttachmentReadRequest struct {
	Ref          MessageRef `json:"ref"`
	AttachmentID string     `json:"attachmentId"`
}

// ScopedError associates a safe failure with an account when the failure is
// limited to one account in a cross-account request.
type ScopedError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	AccountID string    `json:"accountId,omitempty"`
	Retryable bool      `json:"retryable"`
}

// Response wraps successful data with request correlation metadata.
type Response[T any] struct {
	Data      T             `json:"data"`
	Errors    []ScopedError `json:"errors,omitempty"`
	RequestID string        `json:"requestId"`
}

// ErrorResponse is the stable JSON error envelope returned by v1 routes.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is safe for an API client to display or process.
type ErrorBody struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
	RequestID string    `json:"requestId"`
}

// FetchMode separates cached reads from explicit provider reads.
type FetchMode string

const (
	// FetchCachedOnly returns locally cached content and never contacts a provider.
	FetchCachedOnly FetchMode = "cached"
	// FetchExplicit permits an explicitly requested provider fetch when authorized.
	FetchExplicit FetchMode = "fetch"
)

// IdempotencyKey is supplied by a client for a state-changing operation.
type IdempotencyKey string

// Revision identifies the version of a resource a client last read.
type Revision string

// MutationPrecondition carries the idempotency key and optional optimistic
// revision required by a state-changing operation. The HTTP adapter maps the
// key to the Idempotency-Key header; it is not accepted from a target payload.
type MutationPrecondition struct {
	IdempotencyKey IdempotencyKey `json:"-"`
	Expected       Revision       `json:"expectedRevision,omitempty"`
}

// OperationReceipt identifies a durable mutation or delivery job. State is a
// stable operation state, not a claim that an external provider accepted mail.
type OperationReceipt struct {
	OperationID string         `json:"operationId"`
	State       OperationState `json:"state"`
	Replayed    bool           `json:"replayed,omitempty"`
}

// OperationState describes the current durable state of a mutation or send.
type OperationState string

const (
	// OperationQueued is durably accepted and waiting for its due time.
	OperationQueued OperationState = "queued"
	// OperationRetry means delivery is safe to retry because non-acceptance is known.
	OperationRetry OperationState = "retry"
	// OperationSending means the provider operation is currently in progress.
	OperationSending OperationState = "sending"
	// OperationSent means delivery was confirmed by the provider transport.
	OperationSent OperationState = "sent"
	// OperationCancelled means the operation was cancelled before delivery.
	OperationCancelled OperationState = "cancelled"
	// OperationFailed means a permanent failure was confirmed.
	OperationFailed OperationState = "failed"
	// OperationUnknown means provider acceptance cannot be safely determined.
	OperationUnknown OperationState = "outcome_unknown"
)
