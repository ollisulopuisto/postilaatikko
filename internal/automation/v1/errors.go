package v1

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable machine-readable API failure code.
type ErrorCode string

const (
	// CodeInvalidRequest indicates malformed or out-of-range input.
	CodeInvalidRequest ErrorCode = "invalid_request"
	// CodeUnauthenticated indicates missing or invalid client credentials.
	CodeUnauthenticated ErrorCode = "unauthenticated"
	// CodeCapabilityDenied indicates that the client lacks the required grant.
	CodeCapabilityDenied ErrorCode = "capability_denied"
	// CodeNotFound indicates that the requested local resource does not exist.
	CodeNotFound ErrorCode = "not_found"
	// CodeAccountUnreachable indicates incomplete results for an unavailable account.
	CodeAccountUnreachable ErrorCode = "account_unreachable"
	// CodeProviderError indicates a provider operation failed.
	CodeProviderError ErrorCode = "provider_error"
	// CodeContentNotFetched indicates a cached-only read has no local body.
	CodeContentNotFetched ErrorCode = "content_not_fetched"
	// CodeBodyTooLarge indicates a body or attachment exceeds the endpoint limit.
	CodeBodyTooLarge ErrorCode = "body_too_large"
	// CodeRateLimited indicates the caller must slow down or retry later.
	CodeRateLimited ErrorCode = "rate_limited"
	// CodeIdempotencyConflict indicates a key was reused for a different operation.
	CodeIdempotencyConflict ErrorCode = "idempotency_conflict"
	// CodeRevisionConflict indicates the resource changed since the caller read it.
	CodeRevisionConflict ErrorCode = "revision_conflict"
	// CodeInternal indicates an unexpected failure without exposing its cause.
	CodeInternal ErrorCode = "internal_error"
)

// Fault holds a stable public error and an optional private cause. Serialize
// Response() rather than Error() so implementation details are never exposed.
type Fault struct {
	Code      ErrorCode
	Message   string
	Retryable bool
	Cause     error
}

// Error returns only the stable code, never the internal cause or message.
func (f *Fault) Error() string {
	if f == nil {
		return ""
	}
	return string(f.Code)
}

// Unwrap exposes the internal cause to trusted in-process error handling.
func (f *Fault) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Cause
}

// Response creates the safe JSON envelope for this fault.
func (f *Fault) Response(requestID string) ErrorResponse {
	if f == nil {
		return ErrorResponse{Error: ErrorBody{Code: CodeInternal, Message: "The request failed.", RequestID: requestID}}
	}
	return ErrorResponse{Error: ErrorBody{
		Code:      f.Code,
		Message:   f.Message,
		Retryable: f.Retryable,
		RequestID: requestID,
	}}
}

// NewFault creates a fault with a stable, safe message and an optional cause.
func NewFault(code ErrorCode, message string, retryable bool, cause error) *Fault {
	if code == "" {
		code = CodeInternal
	}
	if message == "" {
		message = "The request failed."
	}
	return &Fault{Code: code, Message: message, Retryable: retryable, Cause: cause}
}

// IsCode reports whether err wraps a fault with the given public code.
func IsCode(err error, code ErrorCode) bool {
	var fault *Fault
	return errors.As(err, &fault) && fault.Code == code
}

// InvalidRequest constructs a non-retryable validation fault.
func InvalidRequest(message string, cause error) error {
	return NewFault(CodeInvalidRequest, message, false, cause)
}

// InternalError wraps an internal cause without exposing its details.
func InternalError(cause error) error {
	if cause == nil {
		cause = fmt.Errorf("unspecified internal failure")
	}
	return NewFault(CodeInternal, "The request could not be completed.", false, cause)
}
