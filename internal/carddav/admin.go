package carddav

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/logging"
)

// SourceCredentialStore is the credential surface the shared contact-source
// admin operations need. *credentials.Store satisfies it; both the desktop
// app and the headless helper run these functions so local and remote modes
// apply identical semantics.
type SourceCredentialStore interface {
	SetCardDAVPassword(sourceID, password string) error
	DeleteCardDAVPassword(sourceID string) error
	GetContactSourceOAuthTokens(sourceID string) (*credentials.OAuthTokens, error)
	SetContactSourceOAuthTokens(sourceID string, tokens *credentials.OAuthTokens) error
	DeleteContactSourceOAuthTokens(sourceID string) error
}

// AddressbookDisplayName resolves the stored display name for an enabled
// addressbook path, falling back to the path's last segment.
func AddressbookDisplayName(names map[string]string, path string) string {
	if name := names[path]; name != "" {
		return name
	}
	if parts := strings.Split(strings.Trim(path, "/"), "/"); len(parts) > 0 && parts[len(parts)-1] != "" {
		return parts[len(parts)-1]
	}
	return path
}

// CreateSourceAdmin creates a contact source, stores its password and creates
// the enabled addressbooks. Sync triggering is left to the caller.
func CreateSourceAdmin(store *Store, creds SourceCredentialStore, config SourceConfig) (*Source, error) {
	log := logging.WithComponent("carddav.admin")
	source, err := store.CreateSource(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}
	if config.Password != "" {
		if err := creds.SetCardDAVPassword(source.ID, config.Password); err != nil {
			_ = store.DeleteSource(source.ID)
			return nil, fmt.Errorf("failed to store password: %w", err)
		}
	}
	for _, path := range config.EnabledAddressbooks {
		if _, err := store.CreateAddressbook(source.ID, path, AddressbookDisplayName(config.AddressbookNames, path), true); err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Failed to create addressbook")
		}
	}
	return source, nil
}

// UpdateSourceAdmin updates a source, rotates its password when provided and
// applies the differential addressbook update (removed paths are deleted with
// their record state; new paths are created; existing paths keep their UUID,
// sync token and cached records).
func UpdateSourceAdmin(store *Store, creds SourceCredentialStore, id string, config SourceConfig) error {
	log := logging.WithComponent("carddav.admin")
	existingSource, err := store.GetSource(id)
	if err != nil {
		return err
	}
	if existingSource != nil && existingSource.Type != SourceTypeCardDAV {
		config.Username = existingSource.Username
	}
	if err := store.UpdateSource(id, &config); err != nil {
		return fmt.Errorf("failed to update source: %w", err)
	}
	if config.Password != "" {
		if err := creds.SetCardDAVPassword(id, config.Password); err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}
	}
	if len(config.EnabledAddressbooks) > 0 {
		existing, listErr := store.ListAddressbooks(id)
		if listErr != nil {
			return fmt.Errorf("failed to list current addressbooks: %w", listErr)
		}
		existingByPath := make(map[string]*Addressbook, len(existing))
		for _, ab := range existing {
			existingByPath[ab.Path] = ab
		}
		incomingByPath := make(map[string]bool, len(config.EnabledAddressbooks))
		for _, path := range config.EnabledAddressbooks {
			incomingByPath[path] = true
		}
		for path, ab := range existingByPath {
			if incomingByPath[path] {
				continue
			}
			if err := store.DeleteAddressbookByID(ab.ID); err != nil {
				log.Warn().Err(err).Str("path", path).Msg("Failed to delete removed addressbook")
			}
		}
		for _, path := range config.EnabledAddressbooks {
			if _, exists := existingByPath[path]; exists {
				continue
			}
			if _, err := store.CreateAddressbook(id, path, AddressbookDisplayName(config.AddressbookNames, path), true); err != nil {
				log.Warn().Err(err).Str("path", path).Msg("Failed to create new addressbook")
			}
		}
	}
	return nil
}

// DeleteSourceAdmin deletes a source and its credentials. Callers hold any
// higher-level lock they need (the desktop serializes with contactTokenMu).
func DeleteSourceAdmin(store *Store, creds SourceCredentialStore, id string) error {
	source, _ := store.GetSource(id)
	if err := store.DeleteSource(id); err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}
	if source != nil {
		switch source.Type {
		case SourceTypeCardDAV:
			_ = creds.DeleteCardDAVPassword(id)
		case SourceTypeGoogle, SourceTypeMicrosoft:
			// Source credentials are always independent of Mail, including
			// linked Google sources.
			_ = creds.DeleteContactSourceOAuthTokens(id)
		}
	}
	return nil
}

// ForceSyncAdmin clears per-addressbook sync tokens so the next sync
// re-fetches every vCard. Callers trigger the sync itself.
func ForceSyncAdmin(store *Store, sourceID string) error {
	abs, err := store.ListAddressbooks(sourceID)
	if err != nil {
		return fmt.Errorf("failed to list addressbooks: %w", err)
	}
	for _, ab := range abs {
		if err := store.UpdateAddressbookSyncToken(ab.ID, ""); err != nil {
			return fmt.Errorf("failed to clear sync token for addressbook %s: %w", ab.ID, err)
		}
	}
	return nil
}

// AuthorizationParams carries the verified outcome of a contact OAuth consent
// flow. The flow itself always runs on an interactive host; both the desktop
// (local mode) and the helper (remote mode, via the contacts.manage route)
// persist the result through SaveAuthorization.
type AuthorizationParams struct {
	SourceID     string   // existing source for reauthorization, empty to create
	AccountID    string   // linked email account (Google only)
	Name         string   // display name for a new source
	SyncInterval int      // minutes
	Provider     string   // oauth2 provider config name, e.g. "google-contacts"
	Email        string   // verified identity from the OAuth callback
	AccessToken  string   //
	RefreshToken string   //
	ExpiresIn    int      // seconds from now
	Scopes       []string // provider config scopes
}

// SaveAuthorization persists the outcome of a contact OAuth consent flow.
// Mirrors the desktop semantics exactly: duplicate-identity rejection,
// Microsoft account unlinking, refresh-token carry-over on Microsoft
// reauthorization, forced full resync on reauthorization, writable reset.
// The caller triggers the follow-up sync.
func SaveAuthorization(store *Store, creds SourceCredentialStore, mu *sync.Mutex, params AuthorizationParams) (*Source, error) {
	mu.Lock()
	defer mu.Unlock()
	provider := strings.TrimSuffix(params.Provider, "-contacts")
	if params.SourceID == "" {
		sources, err := store.ListSources()
		if err != nil {
			return nil, err
		}
		for _, s := range sources {
			if string(s.Type) == provider && strings.EqualFold(s.Username, params.Email) {
				return nil, fmt.Errorf("Contacts for %s already exists; reauthorize the existing source", params.Email)
			}
		}
	}
	// Microsoft Contacts retains the standalone Graph model (Outlook Mail
	// and Graph tokens have different audiences).
	if provider == "microsoft" {
		params.AccountID = ""
	}
	created := params.SourceID == ""
	var source *Source
	var err error
	if created {
		source, err = store.CreateSource(&SourceConfig{
			Name: params.Name, Type: SourceType(provider), Username: params.Email,
			AccountID: params.AccountID, Enabled: true, SyncInterval: params.SyncInterval,
		})
	} else {
		source, err = store.GetSource(params.SourceID)
	}
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("contact source no longer exists")
	}
	tokens := &credentials.OAuthTokens{
		Provider:     params.Provider,
		AccessToken:  params.AccessToken,
		RefreshToken: params.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(params.ExpiresIn) * time.Second),
		Scopes:       params.Scopes,
	}
	if tokens.RefreshToken == "" && provider == "microsoft" {
		if previous, err := creds.GetContactSourceOAuthTokens(source.ID); err == nil && previous.Provider == tokens.Provider {
			tokens.RefreshToken = previous.RefreshToken
		}
	}
	if tokens.RefreshToken == "" {
		if created {
			_ = store.DeleteSource(source.ID)
		}
		return nil, fmt.Errorf("Contacts authorization returned no refresh token; sign in again")
	}
	if err := creds.SetContactSourceOAuthTokens(source.ID, tokens); err != nil {
		if created {
			_ = creds.DeleteContactSourceOAuthTokens(source.ID)
			_ = store.DeleteSource(source.ID)
		}
		return nil, fmt.Errorf("save Contacts credentials: %w", err)
	}
	// Reauthorization can change the client/grant behind a legacy source.
	// Force a full provider sync while retaining the cache until it succeeds.
	if !created {
		books, err := store.ListAddressbooks(source.ID)
		if err != nil {
			return nil, err
		}
		for _, book := range books {
			if err := store.UpdateAddressbookSyncToken(book.ID, ""); err != nil {
				return nil, err
			}
		}
	}
	if err := store.SetOAuthIdentity(source.ID, params.Email); err != nil {
		return nil, err
	}
	// A fresh read-only grant does not retain a previous write capability.
	if err := store.SetSourceWritable(source.ID, false); err != nil {
		return nil, err
	}
	source.Username, source.Writable = params.Email, false
	return source, nil
}
