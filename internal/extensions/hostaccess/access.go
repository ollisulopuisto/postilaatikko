// Package hostaccess provides in-process extension-scoped adapters usable by
// both desktop and headless hosts. It is not an HTTP or secret-export API.
package hostaccess

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
)

var validID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Broker is the existing OAuth broker surface, not a token retrieval interface.
type Broker interface {
	HTTPClientForExtension(string, coreapi.Manifest, string, []coreapi.AuthScope) (*http.Client, error)
	IMAPClient(string, []string) (coreapi.IMAPClient, error)
	SMTPClient(string) (coreapi.SMTPClient, error)
}

// Auth pins a trusted host-supplied manifest and extension identity. A caller
// must never construct it from a remote request's claimed identity/manifest.
type Auth struct {
	broker      Broker
	extensionID string
	manifest    coreapi.Manifest
	consent     func(coreapi.StartIncrementalConsentRequest) error
}

// NewAuth binds Auth to a trusted compiled manifest and optional host consent UI.
func NewAuth(broker Broker, extensionID string, manifest coreapi.Manifest, consent func(coreapi.StartIncrementalConsentRequest) error) (*Auth, error) {
	if broker == nil || !validID.MatchString(extensionID) || manifest.ID != extensionID {
		return nil, errors.New("extension Auth requires a broker and matching trusted manifest identity")
	}
	return &Auth{broker: broker, extensionID: extensionID, manifest: cloneManifest(manifest), consent: consent}, nil
}
func cloneManifest(m coreapi.Manifest) coreapi.Manifest {
	m.Capabilities = append([]string(nil), m.Capabilities...)
	if m.OAuth != nil {
		copy := *m.OAuth
		copy.FirstPartyUsesCoreForScopes = append([]string(nil), m.OAuth.FirstPartyUsesCoreForScopes...)
		m.OAuth = &copy
	}
	return m
}

// HTTPClient requests an account client with the pinned extension identity and scopes.
func (a *Auth) HTTPClient(accountID string, scopes []coreapi.AuthScope) (*http.Client, error) {
	if a == nil || a.broker == nil {
		return nil, errors.New("extension Auth is unavailable")
	}
	return a.broker.HTTPClientForExtension(a.extensionID, cloneManifest(a.manifest), accountID, append([]coreapi.AuthScope(nil), scopes...))
}

// IMAPClient delegates the account IMAP connection through the host broker.
func (a *Auth) IMAPClient(accountID string, caps []string) (coreapi.IMAPClient, error) {
	if a == nil || a.broker == nil {
		return nil, errors.New("extension Auth is unavailable")
	}
	return a.broker.IMAPClient(accountID, caps)
}

// SMTPClient delegates the account SMTP connection through the host broker.
func (a *Auth) SMTPClient(accountID string) (coreapi.SMTPClient, error) {
	if a == nil || a.broker == nil {
		return nil, errors.New("extension Auth is unavailable")
	}
	return a.broker.SMTPClient(accountID)
}

// StartIncrementalConsent invokes host UI when available; headless hosts fail closed.
func (a *Auth) StartIncrementalConsent(req coreapi.StartIncrementalConsentRequest) error {
	if a == nil || a.consent == nil {
		return errors.New("interactive extension consent is unavailable on this host; use an explicit device authorization flow")
	}
	return a.consent(req)
}

// SecretStore is implemented by credentials.Store. Namespace is bound once by
// a trusted host, never selected by an individual extension operation.
type SecretStore interface {
	SetExtensionSecret(string, string, string) error
	GetExtensionSecret(string, string) (string, error)
	DeleteExtensionSecret(string, string) error
	DeleteAllExtensionSecrets(string) error
}

// Secrets binds every operation to one trusted extension namespace.
type Secrets struct {
	store       SecretStore
	extensionID string
}

// NewSecrets validates and pins the host-supplied extension namespace.
func NewSecrets(store SecretStore, extensionID string) (*Secrets, error) {
	if store == nil || !validID.MatchString(extensionID) {
		return nil, errors.New("extension Secrets require a store and valid extension identity")
	}
	return &Secrets{store: store, extensionID: extensionID}, nil
}

// Set writes a value inside the pinned extension namespace.
func (s *Secrets) Set(key, value string) error {
	if s == nil || s.store == nil {
		return errors.New("extension Secrets unavailable")
	}
	return s.store.SetExtensionSecret(s.extensionID, key, value)
}

// Get reads a value only from the pinned extension namespace.
func (s *Secrets) Get(key string) (string, error) {
	if s == nil || s.store == nil {
		return "", errors.New("extension Secrets unavailable")
	}
	return s.store.GetExtensionSecret(s.extensionID, key)
}

// Delete removes one value from the pinned extension namespace.
func (s *Secrets) Delete(key string) error {
	if s == nil || s.store == nil {
		return errors.New("extension Secrets unavailable")
	}
	return s.store.DeleteExtensionSecret(s.extensionID, key)
}

// DeleteAll removes values only from the pinned extension namespace.
func (s *Secrets) DeleteAll() error {
	if s == nil || s.store == nil {
		return errors.New("extension Secrets unavailable")
	}
	return s.store.DeleteAllExtensionSecrets(s.extensionID)
}

// HostSecrets exposes only the existing read-only CardDAV credential class.
// The lookup callback must be a host-local credential-store operation.
type HostSecrets struct{ cardDAVPassword func(string) (string, error) }

// NewHostSecrets wraps a host-local read-only CardDAV password lookup.
func NewHostSecrets(lookup func(string) (string, error)) *HostSecrets {
	return &HostSecrets{cardDAVPassword: lookup}
}

// Get accepts only validated carddav source keys and never other secret classes.
func (s *HostSecrets) Get(key string) (string, error) {
	if s == nil || s.cardDAVPassword == nil {
		return "", errors.New("host Secrets unavailable")
	}
	if !strings.HasPrefix(key, "carddav:") {
		return "", errors.New("unsupported host secret class")
	}
	id := strings.TrimPrefix(key, "carddav:")
	if id == "" || len(id) > 200 || strings.ContainsAny(id, "/\\\r\n\t ") || strings.Contains(id, "..") {
		return "", errors.New("invalid CardDAV source identity")
	}
	return s.cardDAVPassword(id)
}

var _ coreapi.Auth = (*Auth)(nil)
var _ coreapi.Secrets = (*Secrets)(nil)
var _ coreapi.HostSecrets = (*HostSecrets)(nil)
