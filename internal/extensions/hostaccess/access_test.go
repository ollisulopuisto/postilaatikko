package hostaccess

import (
	"errors"
	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
	"net/http"
	"testing"
)

type fakeBroker struct {
	extension, account string
	manifest           coreapi.Manifest
}

func (b *fakeBroker) HTTPClientForExtension(ext string, m coreapi.Manifest, account string, _ []coreapi.AuthScope) (*http.Client, error) {
	b.extension = ext
	b.account = account
	b.manifest = m
	return &http.Client{}, nil
}
func (*fakeBroker) IMAPClient(string, []string) (coreapi.IMAPClient, error) { return nil, nil }
func (*fakeBroker) SMTPClient(string) (coreapi.SMTPClient, error)           { return nil, nil }
func TestAuthPinsExtensionAndFailsClosedWithoutHost(t *testing.T) {
	broker := &fakeBroker{}
	auth, err := NewAuth(broker, "calendar", coreapi.Manifest{ID: "calendar"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.HTTPClient("account", nil); err != nil {
		t.Fatal(err)
	}
	if broker.extension != "calendar" || broker.account != "account" {
		t.Fatal("scope changed")
	}
	if err := auth.StartIncrementalConsent(coreapi.StartIncrementalConsentRequest{}); err == nil {
		t.Fatal("headless host silently accepted consent")
	}
	if _, err := NewAuth(nil, "calendar", coreapi.Manifest{ID: "calendar"}, nil); err == nil {
		t.Fatal("nil broker accepted")
	}
	if _, err := NewAuth(broker, "calendar", coreapi.Manifest{ID: "contacts"}, nil); err == nil {
		t.Fatal("manifest identity mismatch accepted")
	}
}

type fakeSecrets struct {
	keys    map[string]string
	lastExt string
}

func (s *fakeSecrets) SetExtensionSecret(ext, key, value string) error {
	s.lastExt = ext
	s.keys[ext+":"+key] = value
	return nil
}
func (s *fakeSecrets) GetExtensionSecret(ext, key string) (string, error) {
	s.lastExt = ext
	return s.keys[ext+":"+key], nil
}
func (s *fakeSecrets) DeleteExtensionSecret(ext, key string) error {
	delete(s.keys, ext+":"+key)
	return nil
}
func (s *fakeSecrets) DeleteAllExtensionSecrets(ext string) error {
	for key := range s.keys {
		if key == ext+":one" {
			delete(s.keys, key)
		}
	}
	return nil
}
func TestSecretHandlesCannotSelectAnotherExtension(t *testing.T) {
	store := &fakeSecrets{keys: map[string]string{}}
	a, err := NewSecrets(store, "calendar")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSecrets(store, "contacts")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Set("one", "calendar-secret"); err != nil {
		t.Fatal(err)
	}
	if v, err := b.Get("one"); err != nil || v != "" {
		t.Fatal("cross-extension secret disclosed")
	}
	if err := b.Set("one", "contacts-secret"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if v, _ := b.Get("one"); v != "contacts-secret" {
		t.Fatal("cross-extension bulk deletion")
	}
	if _, err := NewSecrets(nil, "calendar"); err == nil {
		t.Fatal("nil secrets accepted")
	}
	if _, err := NewSecrets(store, ""); err == nil {
		t.Fatal("empty scope accepted")
	}
}
func TestHostSecretsPermitOnlyExplicitCardDAVNamespace(t *testing.T) {
	var calls int
	reader := NewHostSecrets(func(id string) (string, error) {
		calls++
		if id != "source" {
			return "", errors.New("unexpected source")
		}
		return "password", nil
	})
	if got, err := reader.Get("carddav:source"); err != nil || got != "password" {
		t.Fatal("valid read failed")
	}
	for _, key := range []string{"oauth:account", "carddav:", "carddav:../source", "carddav:source\n"} {
		if _, err := reader.Get(key); err == nil {
			t.Fatalf("invalid namespace %q accepted", key)
		}
	}
	if calls != 1 {
		t.Fatal("invalid request reached secret store")
	}
}

func TestAuthManifestIsSnapshotNotMutablePrivilegeInput(t *testing.T) {
	broker := &fakeBroker{}
	manifest := coreapi.Manifest{ID: "calendar", OAuth: &coreapi.ManifestOAuth{FirstPartyUsesCoreForScopes: []string{"original"}}}
	auth, err := NewAuth(broker, "calendar", manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest.OAuth.FirstPartyUsesCoreForScopes[0] = "modified"
	if _, err := auth.HTTPClient("account", nil); err != nil {
		t.Fatal(err)
	}
	if broker.manifest.OAuth.FirstPartyUsesCoreForScopes[0] != "original" {
		t.Fatal("mutable manifest changed routing policy")
	}
	broker.manifest.OAuth.FirstPartyUsesCoreForScopes[0] = "modified-by-broker"
	if _, err := auth.HTTPClient("account", nil); err != nil {
		t.Fatal(err)
	}
	if broker.manifest.OAuth.FirstPartyUsesCoreForScopes[0] != "original" {
		t.Fatal("broker mutated future routing policy")
	}
}
