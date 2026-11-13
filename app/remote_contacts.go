package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/account"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/carddav"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// contactsAPIRequestTimeout is longer than the status-probe timeout because
// contact mutations can trigger provider syncs.
const contactsAPIRequestTimeout = 120 * time.Second

// remoteContactsActive reports whether the desktop is running against a
// configured remote mailbox helper.
func (a *App) remoteContactsActive() bool {
	return a.mailboxService != nil && a.mailboxService.isRemote()
}

// remoteContactsClient returns an automation client for the remote helper
// with a contacts-appropriate timeout. The connector caches the credential;
// this builds a fresh client so contacts calls do not share the short
// status-probe timeout.
func (a *App) remoteContactsClient() (*automationclient.Client, error) {
	if a.mailboxService == nil {
		return nil, errors.New("remote mailbox service is not configured")
	}
	if _, err := a.mailboxService.getClient(); err != nil {
		return nil, err
	}
	a.mailboxService.mu.Lock()
	endpoint, credential := a.mailboxService.endpoint, a.mailboxService.credential
	a.mailboxService.mu.Unlock()
	if credential == nil || credential.Token == "" {
		return nil, errDesktopMailboxAuthorizationRequired
	}
	return newMailboxAPIClientAtWithTimeout(endpoint, credential.Token, contactsAPIRequestTimeout), nil
}

// remoteContactsError maps a client error onto the desktop-facing message.
// A missing capability grant gets an actionable message naming it. Works with
// both the current flattened client errors and the structured v1.Fault shape
// #247 introduces.
func remoteContactsError(err error, capability v1.Capability) error {
	if err == nil {
		return nil
	}
	switch v1.ErrorCode(remoteContactsFaultCode(err)) {
	case v1.CodeCapabilityDenied:
		return fmt.Errorf("the remote helper grant does not allow %s: reconnect the mailbox service with contacts capabilities (Settings → Mailbox service)", capability)
	case v1.CodeConflict, v1.CodeRevisionConflict:
		// The Contacts UI listens for this event to refresh the edited contact.
		return &contactConflictError{cause: err}
	}
	return err
}

// contactConflictError preserves the conflict signal for the Contacts
// frontend; Contacts_UpdateContact callers treat it like the local
// coreapi.ErrConflict (event emitted, edit superseded).
type contactConflictError struct{ cause error }

func (e *contactConflictError) Error() string { return e.cause.Error() }
func (e *contactConflictError) Unwrap() error { return e.cause }

// contactsPendingKeys retains each pending contacts mutation's idempotency
// key by the mutation's canonical request body. The helper's mutation ledger
// records outcomes per key, and binding the key to the exact request keeps
// all three failure modes straight: a retry of the same request after an
// ambiguous transport failure (timeout, dropped connection) replays the
// helper's recorded receipt instead of executing twice; an edit to any
// field is a different request and therefore a new action with its own
// key; and a settled action repeated later (toggle off and back on) is a
// new mutation, never a replay of the stale first outcome.
var contactsPendingKeys = struct {
	sync.Mutex
	byContent map[string]string
}{byContent: make(map[string]string)}

// contactsCanonicalBody renders the exact mutation request as its canonical
// JSON identity (Go marshaling is deterministic: struct order is fixed and
// map keys sort). The pending key is bound to this, so the desktop's notion
// of "same request" always matches the helper's request fingerprint.
// Callers must scrub secrets (CardDAV passwords, OAuth tokens) before
// marshaling: a fast unsalted hash - or a retained plaintext copy - of a
// password is a credential leak risk, and the identity does not need it. A
// secret-only change while the previous attempt is still pending surfaces
// as the helper's idempotency conflict, which is the correct answer while
// that attempt's outcome is unknown.
func contactsCanonicalBody(request any) string {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Sprintf("%#v", request)
	}
	return string(body)
}

// maxContactsPendingKeys bounds retained pending mutations. Beyond it, new
// mutations run without receipt protection rather than growing memory
// without limit during a helper outage.
const maxContactsPendingKeys = 64

// contactsPendingContentKey digests the request identity to a fixed size:
// canonical bodies carry inline base64 photos, and retaining the raw bodies
// for transport-failed mutations would accumulate photo data for the
// process lifetime. Passwords are already scrubbed before the body reaches
// here, so a fast hash is safe for what remains.
func contactsPendingContentKey(operation, canonicalBody string) string {
	sum := sha256.Sum256([]byte(operation + "\x00" + canonicalBody))
	return hex.EncodeToString(sum[:])
}

// contactsIdempotencyKey returns the retained key for this exact pending
// request, creating and retaining a fresh one when it has none.
func contactsIdempotencyKey(operation, canonicalBody string) string {
	content := contactsPendingContentKey(operation, canonicalBody)
	contactsPendingKeys.Lock()
	defer contactsPendingKeys.Unlock()
	if key, ok := contactsPendingKeys.byContent[content]; ok {
		return key
	}
	key := "desktop-contacts-" + operation + "-" + uuid.NewString()
	if len(contactsPendingKeys.byContent) < maxContactsPendingKeys {
		contactsPendingKeys.byContent[content] = key
	}
	return key
}

// settleContactsMutation drops the retained key once the mutation's outcome
// is known: success or a definitive structured error. Transport-level
// failures (no fault code) and idempotency conflicts (the helper reporting
// the same in-flight or receipt-uncertain operation) keep the key, so a
// retry of the same request replays the helper's receipt instead of
// executing twice.
func settleContactsMutation(err error, operation, canonicalBody string) {
	if err != nil {
		code := remoteContactsFaultCode(err)
		if code == "" || v1.ErrorCode(code) == v1.CodeIdempotencyConflict {
			return
		}
	}
	content := contactsPendingContentKey(operation, canonicalBody)
	contactsPendingKeys.Lock()
	delete(contactsPendingKeys.byContent, content)
	contactsPendingKeys.Unlock()
}

// contactsExtensionEnabled mirrors the Contacts bridge's enabled gate so
// remote calls honor the same setting the bridge checks locally.
func (a *App) contactsExtensionEnabled() bool {
	if a.settingsStore == nil {
		return false
	}
	enabled, err := a.settingsStore.IsExtensionEnabled("contacts")
	return err == nil && enabled
}

// remoteContactsFaultCode extracts the stable error code from a client
// error, whether it arrived as a structured v1.Fault (after #247) or as the
// flattened "api error (code): message" form.
func remoteContactsFaultCode(err error) string {
	if err == nil {
		return ""
	}
	var fault *v1.Fault
	if errors.As(err, &fault) {
		return string(fault.Code)
	}
	if msg := err.Error(); strings.HasPrefix(msg, "api error (") {
		if rest := strings.TrimPrefix(msg, "api error ("); strings.Contains(rest, "):") {
			return rest[:strings.Index(rest, "):")]
		}
	}
	return ""
}

// isRemoteContactsConflict reports whether the helper answered a contact
// mutation with a conflict the local bridge would have turned into a
// contacts:conflict event.
func isRemoteContactsConflict(err error) bool {
	switch v1.ErrorCode(remoteContactsFaultCode(err)) {
	case v1.CodeConflict, v1.CodeRevisionConflict:
		return true
	}
	return false
}

// emitRemoteContactsConflict mirrors ContactsBridge.emitConflict: the user's
// edit was superseded by the server, so the UI refreshes instead of failing.
func (a *App) emitRemoteContactsConflict(contactID string, err error) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "contacts:conflict", map[string]string{
		"contactId": contactID,
		"message":   err.Error(),
	})
}

// contactOAuthProviderForAccount infers the contacts OAuth provider from the
// account the same way the local picker does: Gmail hosts are Google,
// Outlook hosts are Microsoft. Remote mode cannot read the credential
// store's provider label, so the IMAP host is the signal.
func contactOAuthProviderForAccount(imapHost string) (string, error) {
	host := strings.ToLower(imapHost)
	switch {
	case strings.Contains(host, "gmail"):
		return "google", nil
	case strings.Contains(host, "outlook"):
		return "microsoft", nil
	}
	return "", fmt.Errorf("contacts sync can only link Google or Microsoft accounts")
}

// linkAccountContactSourceRemote mirrors LinkAccountContactSource: Mail is an
// identity hint, never a token source; the explicit user action opens
// separate Contacts consent on the desktop, and persistence goes through the
// helper (saveContactAuthorization branches remote). Duplicate detection
// happens on the helper inside SaveAuthorization.
func (a *App) linkAccountContactSourceRemote(accountID, name string, syncInterval int) (*carddav.Source, error) {
	acc, err := a.GetAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.AuthType != account.AuthOAuth2 {
		return nil, fmt.Errorf("an OAuth email account is required")
	}
	provider, err := contactOAuthProviderForAccount(acc.IMAPHost)
	if err != nil {
		return nil, err
	}
	// Mirror the local path: an existing account-linked source without its
	// own Contacts tokens is a legacy source to migrate in place (keeping its
	// cached records), not a duplicate to reject. The helper knows the token
	// state; the desktop's stores are nil in remote mode.
	client, err := a.remoteContactsClient()
	if err != nil {
		return nil, err
	}
	sources, err := client.ListContactSources(a.ctx)
	if err != nil {
		return nil, remoteContactsError(err, v1.CapabilityContactsRead)
	}
	sourceID := ""
	for _, candidate := range sources {
		if candidate.AccountID == nil || *candidate.AccountID != accountID {
			continue
		}
		if candidate.HasTokens {
			return nil, fmt.Errorf("account already has a Contacts source; reauthorize that source in Contacts settings")
		}
		sourceID = candidate.ID
		break
	}
	session, err := a.beginContactOAuth(provider, acc.Email)
	if err != nil {
		return nil, err
	}
	defer a.finishContactOAuth(session)
	if err := a.waitContactOAuth(session, acc.Email); err != nil {
		return nil, err
	}
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager != session.manager {
		return nil, fmt.Errorf("Contacts sign-in was cancelled")
	}
	return a.saveContactAuthorization(session, sourceID, accountID, name, syncInterval)
}

// reauthorizeContactSourceRemote mirrors ReauthorizeContactSource: run the
// provider flow on the desktop against the source's verified identity, then
// persist through the helper.
func (a *App) reauthorizeContactSourceRemote(sourceID string) error {
	sources, err := a.GetContactSources()
	if err != nil {
		return err
	}
	var source *carddav.Source
	for _, candidate := range sources {
		if candidate.ID == sourceID {
			source = candidate
			break
		}
	}
	if source == nil {
		return fmt.Errorf("contact source not found")
	}
	email := source.Username
	if email == "" && source.AccountID != nil {
		acc, err := a.GetAccount(*source.AccountID)
		if err != nil {
			return err
		}
		if acc != nil {
			email = acc.Email
		}
	}
	session, err := a.beginContactOAuth(string(source.Type), email)
	if err != nil {
		return err
	}
	defer a.finishContactOAuth(session)
	if err := a.waitContactOAuth(session, email); err != nil {
		return err
	}
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager != session.manager {
		return fmt.Errorf("Contacts sign-in was cancelled")
	}
	accountID := ""
	if source.AccountID != nil {
		accountID = *source.AccountID
	}
	_, err = a.saveContactAuthorization(session, source.ID, accountID, source.Name, source.SyncInterval)
	return err
}
