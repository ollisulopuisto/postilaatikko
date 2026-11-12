package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/carddav"
	"github.com/hkdb/aerion/internal/kit/davutil"
	"github.com/hkdb/aerion/internal/logging"
)

// LinkedAccountInfo represents an email account that can be linked to a contact source
type LinkedAccountInfo struct {
	AccountID       string `json:"accountId"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`        // "google" or "microsoft"
	IsLinked        bool   `json:"isLinked"`        // Already has contact source linked
	ContactSourceID string `json:"contactSourceId"` // Separate Contacts authorization, never a Mail scope check
}

// ============================================================================
// CardDAV Contact Source API - Exposed to frontend via Wails bindings
// ============================================================================

// DiscoverCardDAVAddressbooks discovers available addressbooks from a CardDAV server
func (a *App) DiscoverCardDAVAddressbooks(url, username, password string) ([]carddav.AddressbookInfo, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		books, err := client.DiscoverContactAddressbooks(a.ctx, v1.ContactsDiscoverRequest{URL: url, Username: username, Password: password})
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsManage)
		}
		return books, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return carddav.DiscoverAddressbooks(url, username, password)
}

// TestCardDAVConnection tests connection to a CardDAV server
func (a *App) TestCardDAVConnection(url, username, password string) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		return remoteContactsError(client.TestContactConnection(a.ctx, v1.ContactsDiscoverRequest{URL: url, Username: username, Password: password}), v1.CapabilityContactsManage)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return carddav.TestConnection(url, username, password)
}

// DiscoverCardDAVAddressbooksOAuth discovers addressbooks from a CardDAV server using
// a bearer token from an OAuth mail account (unified-grant path — a custom OIDC account
// whose token also authorizes CardDAV, e.g. Stalwart). The account token getter is
// already custom-OAuth-aware and refreshing; discovery doubles as the connection test.
func (a *App) DiscoverCardDAVAddressbooksOAuth(url, accountID string) ([]carddav.AddressbookInfo, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		books, err := client.DiscoverContactAddressbooks(a.ctx, v1.ContactsDiscoverRequest{URL: url, AccountID: accountID})
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsManage)
		}
		return books, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	tokens, err := a.getValidOAuthToken(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth token: %w", err)
	}
	httpClient := davutil.NewBearerHTTPClient(tokens.AccessToken, 30*time.Second)
	return carddav.DiscoverAddressbooksWithHTTPClient(url, httpClient)
}

// GetContactSources returns all configured contact sources
func (a *App) GetContactSources() ([]*carddav.Source, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		sources, err := client.ListContactSources(a.ctx)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		out := make([]*carddav.Source, 0, len(sources))
		for i := range sources {
			src := sources[i]
			out = append(out, &src)
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.carddavStore.ListSources()
}

// GetContactSource returns a single contact source by ID
func (a *App) GetContactSource(id string) (*carddav.Source, error) {
	if a.remoteContactsActive() {
		sources, err := a.GetContactSources()
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			if source.ID == id {
				return source, nil
			}
		}
		return nil, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.carddavStore.GetSource(id)
}

// AddContactSource creates a new contact source with addressbooks
func (a *App) AddContactSource(config carddav.SourceConfig) (*carddav.Source, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		source, err := client.MutateContactSource(a.ctx, v1.ContactsSourceMutationRequest{Operation: "create", Config: &config}, contactsIdempotencyKey("source_create", config.Name, string(config.Type), config.URL, config.Username))
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsManage)
		}
		return source, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	log := logging.WithComponent("app")

	source, err := carddav.CreateSourceAdmin(a.carddavStore, a.credStore, config)
	if err != nil {
		return nil, err
	}

	// Trigger initial sync
	go a.carddavSyncer.SyncSource(source.ID)

	log.Info().Str("id", source.ID).Str("name", source.Name).Msg("Contact source created")
	return source, nil
}

// UpdateContactSource updates an existing contact source
func (a *App) UpdateContactSource(id string, config carddav.SourceConfig) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContactSource(a.ctx, v1.ContactsSourceMutationRequest{Operation: "update", ID: id, Config: &config}, contactsIdempotencyKey("source_update", id, config.Name, config.URL, config.Username, fmt.Sprint(config.Writable, config.Enabled, config.SyncInterval)))
		return remoteContactsError(err, v1.CapabilityContactsManage)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	log := logging.WithComponent("app")

	if err := carddav.UpdateSourceAdmin(a.carddavStore, a.credStore, id, config); err != nil {
		return err
	}

	// Trigger resync
	go a.carddavSyncer.SyncSource(id)

	log.Info().Str("id", id).Msg("Contact source updated")
	return nil
}

// DeleteContactSource deletes a contact source and all its data
func (a *App) DeleteContactSource(id string) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContactSource(a.ctx, v1.ContactsSourceMutationRequest{Operation: "delete", ID: id}, contactsIdempotencyKey("source_delete", id))
		return remoteContactsError(err, v1.CapabilityContactsManage)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	a.contactTokenMu.Lock()
	defer a.contactTokenMu.Unlock()
	log := logging.WithComponent("app")

	if err := carddav.DeleteSourceAdmin(a.carddavStore, a.credStore, id); err != nil {
		return err
	}

	log.Info().Str("id", id).Msg("Contact source deleted")
	return nil
}

// GetSourceAddressbooks returns all addressbooks for a source
func (a *App) GetSourceAddressbooks(sourceID string) ([]*carddav.Addressbook, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		books, err := client.ListContactSourceAddressbooks(a.ctx, sourceID)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		out := make([]*carddav.Addressbook, 0, len(books))
		for i := range books {
			b := books[i]
			out = append(out, &b)
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.carddavStore.ListAddressbooks(sourceID)
}

// SetAddressbookEnabled enables or disables an addressbook
func (a *App) SetAddressbookEnabled(addressbookID string, enabled bool) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, v1.ContactsMutationRequest{Operation: "set_addressbook_enabled", AddressbookID: addressbookID, Enabled: enabled}, contactsIdempotencyKey("ab_enabled", addressbookID, fmt.Sprint(enabled)))
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.carddavStore.SetAddressbookEnabled(addressbookID, enabled)
}

// SetContactSourceWritable flips the writable flag for a source that supports
// writes. Google Contacts sources are permanently read-only.
func (a *App) SetContactSourceWritable(sourceID string, writable bool) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, v1.ContactsMutationRequest{Operation: "set_writable", ID: sourceID, Writable: writable}, contactsIdempotencyKey("writable", sourceID, fmt.Sprint(writable)))
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	if writable {
		source, err := a.carddavStore.GetSource(sourceID)
		if err != nil {
			return err
		}
		if source != nil && source.Type == carddav.SourceTypeGoogle {
			return fmt.Errorf("Google Contacts is read-only")
		}
	}
	return a.carddavStore.SetSourceWritable(sourceID, writable)
}

// SyncContactSource manually triggers a sync for a source
func (a *App) SyncContactSource(id string) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, v1.ContactsMutationRequest{Operation: "sync", ID: id}, contactsIdempotencyKey("sync", id, fmt.Sprint(time.Now().Unix()/30)))
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.carddavSyncer.SyncSource(id)
}

// SyncAllContactSources manually triggers a sync for all sources
func (a *App) SyncAllContactSources() error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, v1.ContactsMutationRequest{Operation: "sync_all"}, contactsIdempotencyKey("sync_all", fmt.Sprint(time.Now().Unix()/30)))
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.carddavSyncer.SyncAllSources()
}

// ForceSyncContactSource clears the per-addressbook sync tokens for a
// CardDAV source so the next sync re-fetches every vCard from the
// server. Used to backfill multi-field data (phones, addresses, org,
// notes, etc.) for contacts originally synced under a legacy schema
// where the old parser only stored email + display name. Mirrors
// App.ForceSyncFolder for mail messages.
func (a *App) ForceSyncContactSource(sourceID string) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, v1.ContactsMutationRequest{Operation: "force_sync", ID: sourceID}, contactsIdempotencyKey("force_sync", sourceID, fmt.Sprint(time.Now().Unix()/30)))
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	if err := carddav.ForceSyncAdmin(a.carddavStore, sourceID); err != nil {
		return err
	}
	return a.carddavSyncer.SyncSource(sourceID)
}

// GetContactSourceErrors returns all sources that have errors
func (a *App) GetContactSourceErrors() ([]*carddav.SourceError, error) {
	if a.remoteContactsActive() {
		sources, err := a.GetContactSources()
		if err != nil {
			return nil, err
		}
		var out []*carddav.SourceError
		for _, source := range sources {
			if source.LastError == "" {
				continue
			}
			errAt := time.Now()
			if source.LastErrorAt != nil {
				errAt = *source.LastErrorAt
			}
			out = append(out, &carddav.SourceError{SourceID: source.ID, SourceName: source.Name, Error: source.LastError, ErrorAt: errAt})
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.carddavStore.GetSourcesWithErrors()
}

// GetContactSourceStats returns statistics for contact sources
func (a *App) GetContactSourceStats() (map[string]interface{}, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		stats, err := client.GetContactStats(a.ctx)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		return map[string]interface{}{
			"total_sources":       stats.TotalSources,
			"total_contacts":      stats.TotalContacts,
			"sources_with_errors": stats.SourcesWithErrors,
		}, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	sources, err := a.carddavStore.ListSources()
	if err != nil {
		return nil, err
	}

	totalContacts, _ := a.carddavStore.CountContacts()
	sourcesWithErrors, _ := a.carddavStore.GetSourcesWithErrors()

	return map[string]interface{}{
		"total_sources":       len(sources),
		"total_contacts":      totalContacts,
		"sources_with_errors": len(sourcesWithErrors),
	}, nil
}

// ============================================================================
// OAuth Contact Source API - Google/Microsoft contact sync
// ============================================================================

// GetLinkedAccountsForContactSync returns email accounts that can be linked to contact sources
func (a *App) GetLinkedAccountsForContactSync() ([]LinkedAccountInfo, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		accounts, err := client.ListContactLinkedAccounts(a.ctx, "google")
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		out := make([]LinkedAccountInfo, 0, len(accounts))
		for _, acc := range accounts {
			out = append(out, LinkedAccountInfo{AccountID: acc.AccountID, Email: acc.Email, Name: acc.Name, Provider: acc.Provider, IsLinked: acc.IsLinked, ContactSourceID: acc.ContactSourceID})
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	log := logging.WithComponent("app")

	accounts, err := a.accountStore.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	// Get existing contact sources to check which accounts are already linked
	sources, err := a.carddavStore.ListSources()
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	var result []LinkedAccountInfo
	for _, acc := range accounts {
		// Only OAuth accounts (Google/Microsoft) can be linked for contacts
		if acc.AuthType != account.AuthOAuth2 {
			continue
		}

		// Get OAuth provider
		provider, err := a.credStore.GetOAuthProvider(acc.ID)
		if err != nil || provider == "" {
			continue
		}

		// Only Google supports linked contact sync
		// Microsoft can't share tokens between Outlook (email) and Graph API (contacts) due to audience restrictions
		if provider != "google" {
			continue
		}

		contactSourceID := ""
		for _, source := range sources {
			if source.Type == carddav.SourceTypeGoogle && ((source.AccountID != nil && *source.AccountID == acc.ID) || strings.EqualFold(source.Username, acc.Email)) {
				contactSourceID = source.ID
				break
			}
		}

		result = append(result, LinkedAccountInfo{
			AccountID:       acc.ID,
			Email:           acc.Email,
			Name:            acc.Name,
			Provider:        provider,
			IsLinked:        contactSourceID != "",
			ContactSourceID: contactSourceID,
		})
	}

	log.Debug().Int("count", len(result)).Msg("Found linkable accounts for contact sync")
	return result, nil
}

// GetCustomOAuthAccounts returns mail accounts that authenticate via a custom
// ("bring your own app") OIDC provider. Their access token can be reused to
// authenticate a CardDAV source against the same unified server (e.g. Stalwart),
// so these populate the OAuth-account picker in the Add CardDAV Source dialog.
// Unlike GetLinkedAccountsForContactSync (Google People API), this is the DAV path.
func (a *App) GetCustomOAuthAccounts() ([]LinkedAccountInfo, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		accounts, err := client.ListContactLinkedAccounts(a.ctx, "custom-oauth")
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		out := make([]LinkedAccountInfo, 0, len(accounts))
		for _, acc := range accounts {
			out = append(out, LinkedAccountInfo{AccountID: acc.AccountID, Email: acc.Email, Name: acc.Name, Provider: acc.Provider, IsLinked: acc.IsLinked, ContactSourceID: acc.ContactSourceID})
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	accounts, err := a.accountStore.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	sources, err := a.carddavStore.ListSources()
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}
	linkedAccountIDs := make(map[string]bool)
	for _, source := range sources {
		if source.AccountID != nil && *source.AccountID != "" {
			linkedAccountIDs[*source.AccountID] = true
		}
	}

	var result []LinkedAccountInfo
	for _, acc := range accounts {
		if acc.AuthType != account.AuthOAuth2 {
			continue
		}
		provider, perr := a.credStore.GetOAuthProvider(acc.ID)
		if perr != nil || provider != customOAuthProviderName {
			continue
		}
		result = append(result, LinkedAccountInfo{
			AccountID: acc.ID,
			Email:     acc.Email,
			Name:      acc.Name,
			Provider:  provider,
			IsLinked:  linkedAccountIDs[acc.ID],
		})
	}
	return result, nil
}

// addressbookDisplayName resolves the stored name for an addressbook path:
// the display name discovery found (passed through SourceConfig, #366),
// falling back to the path's last segment.
func addressbookDisplayName(names map[string]string, path string) string {
	return carddav.AddressbookDisplayName(names, path)
}
