package gmail

import (
	"errors"
	"fmt"
)

// Sentinel errors let callers pick a fallback without parsing API payloads.
var (
	// ErrNotCapable means the account cannot use the Gmail API (not a Google
	// OAuth account, or the granted scopes do not cover the call). Callers fall
	// back to IMAP silently.
	ErrNotCapable = errors.New("gmail: account is not capable of Gmail API access")
	// ErrReauthRequired means the access token was rejected even after a forced
	// refresh. IMAP uses the same token, so callers must surface re-authorization.
	ErrReauthRequired = errors.New("gmail: re-authorization required")
	// ErrInsufficientScope means the granted OAuth scopes do not cover the call.
	ErrInsufficientScope = errors.New("gmail: insufficient OAuth scope")
	// ErrAPIDisabled means the Gmail API is not enabled in the Google Cloud
	// project behind the OAuth client.
	ErrAPIDisabled = errors.New("gmail: Gmail API is not enabled for the OAuth client project")
	// ErrRateLimited means the request was still rate limited after retries.
	ErrRateLimited = errors.New("gmail: rate limited")
	// ErrTransient means a server or network error persisted after retries.
	ErrTransient = errors.New("gmail: temporary server or network error")
	// ErrNotFound means the message or label no longer exists.
	ErrNotFound = errors.New("gmail: not found")
	// ErrBadRequest means the API rejected the request, for example an invalid query.
	ErrBadRequest = errors.New("gmail: bad request")
)

// APIError carries the HTTP status and Google's error detail. It unwraps to the
// matching sentinel so errors.Is works.
type APIError struct {
	Status  int
	Reason  string
	Message string
	kind    error
}

// Error implements error.
func (e *APIError) Error() string {
	return fmt.Sprintf("gmail API %d (%s): %s", e.Status, e.Reason, e.Message)
}

// Unwrap returns the classified sentinel, if any.
func (e *APIError) Unwrap() error { return e.kind }
