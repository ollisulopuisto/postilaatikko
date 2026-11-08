package app

import (
	extcontactsbe "github.com/hkdb/aerion/extensions/contacts/backend"
	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Contacts_* methods are explicitly promoted on App by the embedded bridge.
// These wrappers keep that Wails surface safe in remote mode, where the bridge
// is deliberately not initialized because Contacts remains device-local.
// Contacts_ListContactsForBrowse searches and pages extension contacts.
func (a *App) Contacts_ListContactsForBrowse(query, sourceID string, limit, offset int) ([]coreapi.Contact, error) {
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.ContactsBridge.Contacts_ListContactsForBrowse(query, sourceID, limit, offset)
}

// Contacts_GetContactDetail returns one contact from the Contacts extension.
func (a *App) Contacts_GetContactDetail(emailOrID string) (*coreapi.Contact, error) {
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.ContactsBridge.Contacts_GetContactDetail(emailOrID)
}

// Contacts_CreateContact creates a local or CardDAV contact.
func (a *App) Contacts_CreateContact(input coreapi.ContactCreateInput) (string, error) {
	if err := a.requireLocalContacts(); err != nil {
		return "", err
	}
	return a.ContactsBridge.Contacts_CreateContact(input)
}

// Contacts_ListAddressbooks lists addressbooks for a contact source.
func (a *App) Contacts_ListAddressbooks(sourceID string) ([]coreapi.Addressbook, error) {
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.ContactsBridge.Contacts_ListAddressbooks(sourceID)
}

// Contacts_ListSources lists configured contact sources.
func (a *App) Contacts_ListSources() ([]coreapi.ContactSource, error) {
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.ContactsBridge.Contacts_ListSources()
}

// Contacts_LinkAccountSource creates a contact source for an email account.
func (a *App) Contacts_LinkAccountSource(accountID, name string, syncInterval int) (string, error) {
	if err := a.requireLocalContacts(); err != nil {
		return "", err
	}
	return a.ContactsBridge.Contacts_LinkAccountSource(accountID, name, syncInterval)
}

// Contacts_SyncSource syncs one contact source.
func (a *App) Contacts_SyncSource(sourceID string) error {
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.ContactsBridge.Contacts_SyncSource(sourceID)
}

// Contacts_SyncAllSources syncs every configured contact source.
func (a *App) Contacts_SyncAllSources() error {
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.ContactsBridge.Contacts_SyncAllSources()
}

// Contacts_UpdateContact updates a contact in its source.
func (a *App) Contacts_UpdateContact(idOrEmail string, patch coreapi.ContactPatch) error {
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.ContactsBridge.Contacts_UpdateContact(idOrEmail, patch)
}

// Contacts_DeleteLocalContact deletes a contact from its source.
func (a *App) Contacts_DeleteLocalContact(idOrEmail string) error {
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.ContactsBridge.Contacts_DeleteLocalContact(idOrEmail)
}

// Contacts_EnableWriteAccess authorizes writable access to a contact source.
func (a *App) Contacts_EnableWriteAccess(sourceID, authContextKind, authContextIdentifier, expectedEmail string) error {
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.ContactsBridge.Contacts_EnableWriteAccess(sourceID, authContextKind, authContextIdentifier, expectedEmail)
}

// Contacts_ResizeContactPhoto prepares an image for a contact photo.
func (a *App) Contacts_ResizeContactPhoto(b64In string) (extcontactsbe.ResizedContactPhoto, error) {
	if err := a.requireLocalContacts(); err != nil {
		return extcontactsbe.ResizedContactPhoto{}, err
	}
	return a.ContactsBridge.Contacts_ResizeContactPhoto(b64In)
}

// Contacts_ReauthorizeSource renews authorization for a contact source.
func (a *App) Contacts_ReauthorizeSource(sourceID string) error {
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.ContactsBridge.Contacts_ReauthorizeSource(sourceID)
}

// Contacts_CancelSourceAuthorization cancels an in-progress contact OAuth flow.
func (a *App) Contacts_CancelSourceAuthorization() {
	if a.remoteServiceMode {
		return
	}
	a.ContactsBridge.Contacts_CancelSourceAuthorization()
}

// initContactsExtension wires the Contacts extension's Bridge into App
// during Startup. All bridge logic lives in extensions/contacts/backend/
// bridge.go; this file exists ONLY so the host can supply the bridge with
// its host-provided dependencies (settings store, paths, db, event emitter)
// and so the embedded-field promotion makes the bridge methods Wails-bindable.
//
// The bridge lazy-initializes its Contacts-specific state (stores, per-
// extension SQLite, API wrapper) inside `ensureInit` on the first enabled
// method call. When Contacts is disabled in settings, zero work happens
// beyond the ~80-byte Bridge struct allocation — this is how the
// lightweight-by-default promise is held.
func (a *App) initContactsExtension() {
	// Per-extension Core handle for cross-extension coreapi calls (source
	// management via ListSources / LinkAccountSource). Distinct from the
	// Core constructed in the Startup Register loop but functionally
	// equivalent — both point at the same app, scoped to the same
	// extension identity for Auth routing.
	contactsCore := newCoreForExtension(a, a.contactsExt)

	// Single emitter for all contacts:* frontend events (conflict, changed).
	emit := func(eventName string, payload any) {
		wailsRuntime.EventsEmit(a.ctx, eventName, payload)
	}

	a.ContactsBridge = extcontactsbe.NewContactsBridge(extcontactsbe.ContactsBridgeDeps{
		SettingsStore: a.settingsStore,
		Paths:         a.paths,
		DB:            a.db,
		Emitter:       emit,
		// CardDAV passwords flow through Core.Storage().HostSecrets()
		// (Pattern B — core owns the lifecycle; extension reads). No
		// per-credential closure injection needed; the bridge constructs
		// the right key prefix when reading.
		//
		// Core gives the bridge access to host-owned cross-extension
		// surfaces — Contacts().ListSources() and LinkAccountSource()
		// back the sidebar + account-setup hook flows; Storage().HostSecrets()
		// backs CardDAV writes.
		Core: contactsCore,
		// Mirrors what the carddav syncer gets via SetTokenGetters — the
		// same proactively-refreshing token accessor for standalone
		// contacts-only OAuth sources, now reused on the write side so
		// create/update/delete succeed regardless of whether the user
		// linked the source to an email account or set it up via the
		// contacts-only OAuth flow.
		GetStandaloneSourceToken:  a.getValidContactSourceOAuthToken,
		ReauthorizeSource:         a.ReauthorizeContactSource,
		CancelSourceAuthorization: a.CancelContactSourceOAuthFlow,
	})

	// Live-refresh the contact list after any source sync (background
	// scheduler, post-add, or manual). The core carddav syncer fires a generic
	// completion callback; this host-wiring file translates it into the
	// extension's `contacts:changed` frontend event, reusing the same Emitter
	// as contacts:conflict. (carddavSyncer is constructed earlier in Startup.)
	if a.carddavSyncer != nil {
		a.carddavSyncer.SetSyncCompleteHandler(func(sourceID string) {
			emit("contacts:changed", map[string]string{"sourceId": sourceID})
		})
	}

	// All OAuth slot resolution lives in internal/oauth2/core_provider.go
	// now — google-contacts and microsoft-contacts are owned there. The
	// contacts extension package carries no OAuth client vars of its own.
}
