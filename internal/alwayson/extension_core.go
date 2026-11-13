package alwayson

import (
	"errors"

	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
	"github.com/hkdb/aerion/internal/extensions/hostaccess"
)

// extensionCore is the headless coreapi.Core the helper hands to the
// Contacts extension backend. It is scoped by construction through
// hostaccess: Auth is pinned to the contacts manifest and identity, Secrets
// are namespaced to the extension, and HostSecrets exposes only the
// read-only CardDAV credential class. Interactive consent is unavailable on
// the helper; consent flows run on the desktop and persist through the
// contacts.manage routes. Every surface the contacts backend does not use
// returns coreapi.ErrUnimplemented, and there is no generic remote Core or
// secrets-export route.
type extensionCore struct {
	auth     coreapi.Auth
	storage  coreapi.Storage
	contacts coreapi.Contacts
}

func newExtensionCore(auth *hostaccess.Auth, secrets *hostaccess.Secrets, hostSecrets *hostaccess.HostSecrets, contacts coreapi.Contacts) *extensionCore {
	return &extensionCore{
		auth:     auth,
		storage:  &extensionStorage{secrets: secrets, hostSecrets: hostSecrets},
		contacts: contacts,
	}
}

func (c *extensionCore) Mail() coreapi.Mail         { return unimplementedMail{} }
func (c *extensionCore) Composer() coreapi.Composer { return unimplementedComposer{} }
func (c *extensionCore) Contacts() coreapi.Contacts { return c.contacts }
func (c *extensionCore) Auth() coreapi.Auth         { return c.auth }
func (c *extensionCore) Notifications() coreapi.Notifications {
	return unimplementedNotifications{}
}
func (c *extensionCore) UI() coreapi.UI               { return unimplementedUI{} }
func (c *extensionCore) Storage() coreapi.Storage     { return c.storage }
func (c *extensionCore) Events() coreapi.EventBus     { return unimplementedEvents{} }
func (c *extensionCore) Log() coreapi.Logger          { return unimplementedLogger{} }
func (c *extensionCore) HTML() coreapi.HTML           { return unimplementedHTML{} }
func (c *extensionCore) Extension(string) (any, bool) { return nil, false }

// extensionStorage exposes only extension-scoped secrets and the read-only
// CardDAV host credential class; no per-extension KV on the helper.
type extensionStorage struct {
	secrets     *hostaccess.Secrets
	hostSecrets *hostaccess.HostSecrets
}

func (s *extensionStorage) KV(string) coreapi.KVStore { return unimplementedKV{} }
func (s *extensionStorage) Secrets(string) coreapi.Secrets {
	if s.secrets == nil {
		return unimplementedSecrets{}
	}
	return s.secrets
}
func (s *extensionStorage) HostSecrets() coreapi.HostSecrets {
	if s.hostSecrets == nil {
		return unimplementedHostSecrets{}
	}
	return s.hostSecrets
}

var errCoreSurfaceUnavailable = errors.New("this host surface is unavailable on the remote helper")

type unimplementedKV struct{}

func (unimplementedKV) Get(string) (string, error)    { return "", errCoreSurfaceUnavailable }
func (unimplementedKV) Set(string, string) error      { return errCoreSurfaceUnavailable }
func (unimplementedKV) Delete(string) error           { return errCoreSurfaceUnavailable }
func (unimplementedKV) List(string) ([]string, error) { return nil, errCoreSurfaceUnavailable }

type unimplementedSecrets struct{}

func (unimplementedSecrets) Set(string, string) error { return errCoreSurfaceUnavailable }
func (unimplementedSecrets) Get(string) (string, error) {
	return "", errCoreSurfaceUnavailable
}
func (unimplementedSecrets) Delete(string) error { return errCoreSurfaceUnavailable }
func (unimplementedSecrets) DeleteAll() error    { return errCoreSurfaceUnavailable }

type unimplementedHostSecrets struct{}

func (unimplementedHostSecrets) Get(string) (string, error) { return "", errCoreSurfaceUnavailable }

type unimplementedComposer struct{}

func (unimplementedComposer) OpenComposer(coreapi.ComposeRequest) error {
	return errCoreSurfaceUnavailable
}

type unimplementedNotifications struct{}

func (unimplementedNotifications) Show(coreapi.NotifyRequest) error {
	return errCoreSurfaceUnavailable
}

type unimplementedUI struct{}

func (unimplementedUI) RegisterRailTab(coreapi.RailTabRequest) (coreapi.Unregister, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedUI) RegisterSettingsTab(coreapi.SettingsTabRequest) (coreapi.Unregister, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedUI) RegisterContextMenuItem(coreapi.ContextMenuRequest) (coreapi.Unregister, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedUI) RegisterInboxView(coreapi.InboxViewRequest) (coreapi.Unregister, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedUI) RegisterAccountSetupHook(coreapi.AccountSetupHookRequest) (coreapi.Unregister, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedUI) OpenURL(string) error { return errCoreSurfaceUnavailable }

type unimplementedEvents struct{}

func (unimplementedEvents) Subscribe(string, func(payload any)) (coreapi.Unsubscribe, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedEvents) Publish(string, any) error { return errCoreSurfaceUnavailable }

type unimplementedLogger struct{}

func (unimplementedLogger) Debug(string) {}
func (unimplementedLogger) Info(string)  {}
func (unimplementedLogger) Warn(string)  {}
func (unimplementedLogger) Error(string) {}

type unimplementedHTML struct{}

func (unimplementedHTML) Sanitize(string) string { return "" }

var (
	_ coreapi.Core        = (*extensionCore)(nil)
	_ coreapi.Storage     = (*extensionStorage)(nil)
	_ coreapi.KVStore     = unimplementedKV{}
	_ coreapi.Secrets     = unimplementedSecrets{}
	_ coreapi.HostSecrets = unimplementedHostSecrets{}
	_ coreapi.Mail        = unimplementedMail{}
	_ coreapi.Composer    = unimplementedComposer{}
	_ coreapi.UI          = unimplementedUI{}
	_ coreapi.EventBus    = unimplementedEvents{}
	_ coreapi.Logger      = unimplementedLogger{}
	_ coreapi.HTML        = unimplementedHTML{}
)

type unimplementedMail struct{}

func (unimplementedMail) ListMessages(coreapi.MessageFilter) ([]coreapi.Message, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedMail) GetMessage(string, bool) (*coreapi.Message, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedMail) ListFolders(string) ([]coreapi.Folder, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedMail) GetSpecialFolder(string, coreapi.FolderKind) (*coreapi.Folder, error) {
	return nil, errCoreSurfaceUnavailable
}
func (unimplementedMail) MoveMessage(string, string) error { return errCoreSurfaceUnavailable }
func (unimplementedMail) Archive(string) error             { return errCoreSurfaceUnavailable }
func (unimplementedMail) Trash(string) error               { return errCoreSurfaceUnavailable }
func (unimplementedMail) SetFlags(string, coreapi.Flags) error {
	return errCoreSurfaceUnavailable
}
func (unimplementedMail) AppendMessage(string, string, []byte, coreapi.Flags) error {
	return errCoreSurfaceUnavailable
}
func (unimplementedMail) SubscribeToMailEvents([]coreapi.MailEventType) (<-chan coreapi.MailEvent, coreapi.Unsubscribe, error) {
	return nil, nil, errCoreSurfaceUnavailable
}
