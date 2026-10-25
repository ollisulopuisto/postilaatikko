package sync

import (
	"errors"
	"time"

	"github.com/hkdb/aerion/internal/gmail"
)

// GmailAPIState is what the last Gmail API call for an account told us about
// whether the API can be used. It is kept in memory: after a restart an account
// reads as GmailStateUnknown until its first search.
type GmailAPIState string

// Gmail API states shown in account settings.
const (
	GmailStateUnknown        GmailAPIState = "unknown"        // no Gmail API call yet this session
	GmailStateActive         GmailAPIState = "active"         // the last call succeeded
	GmailStateUnavailable    GmailAPIState = "unavailable"    // API disabled in the project, or scope missing
	GmailStateReauthRequired GmailAPIState = "reauthRequired" // the token was rejected; sign in again
	GmailStateNotUsed        GmailAPIState = "notUsed"        // the account cannot use the API (IMAP only)
)

// GmailAPIStatus is the state and the time it was last set.
type GmailAPIStatus struct {
	State GmailAPIState
	Since time.Time
}

// GmailAPIStatus returns the current Gmail API state for an account.
func (e *Engine) GmailAPIStatus(accountID string) GmailAPIStatus {
	e.gmailBlockMu.Lock()
	defer e.gmailBlockMu.Unlock()
	if st, ok := e.gmailStates[accountID]; ok {
		return st
	}
	return GmailAPIStatus{State: GmailStateUnknown}
}

// recordGmailState stores the state implied by one Gmail call's outcome (nil
// for success) and reports whether the state changed. Changes are logged once:
// the first successful call, and every move to a state the user should act on.
// Transient errors keep the previous state, and so do ordinary IMAP-only
// accounts, which only ever report ErrNotCapable and are not logged.
func (e *Engine) recordGmailState(accountID string, err error) bool {
	next, ok := gmailStateFor(err)
	if !ok {
		return false
	}

	e.gmailBlockMu.Lock()
	prev := e.gmailStates[accountID].State
	changed := prev != next.State
	if changed {
		next.Since = time.Now()
		e.gmailStates[accountID] = next
	}
	e.gmailBlockMu.Unlock()

	if !changed || next.State == GmailStateNotUsed {
		return changed
	}
	switch next.State {
	case GmailStateActive:
		e.log.Info().Str("account_id", accountID).Msg("Gmail API search is active for account")
	case GmailStateUnavailable:
		e.log.Warn().Err(err).Str("account_id", accountID).Msg("Gmail API unavailable for account; searching with IMAP")
	case GmailStateReauthRequired:
		e.log.Warn().Err(err).Str("account_id", accountID).Msg("Gmail API needs re-authorization for account")
	}
	return changed
}

func gmailStateFor(err error) (GmailAPIStatus, bool) {
	switch {
	case err == nil:
		return GmailAPIStatus{State: GmailStateActive}, true
	case errors.Is(err, gmail.ErrAPIDisabled), errors.Is(err, gmail.ErrInsufficientScope):
		return GmailAPIStatus{State: GmailStateUnavailable}, true
	case errors.Is(err, gmail.ErrReauthRequired):
		return GmailAPIStatus{State: GmailStateReauthRequired}, true
	case errors.Is(err, gmail.ErrNotCapable):
		return GmailAPIStatus{State: GmailStateNotUsed}, true
	}
	return GmailAPIStatus{}, false
}
