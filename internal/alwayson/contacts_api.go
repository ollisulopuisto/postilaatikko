package alwayson

import (
	"encoding/base64"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hkdb/aerion/extensions/contacts"
	extcontactsbe "github.com/hkdb/aerion/extensions/contacts/backend"
	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/carddav"
	"github.com/hkdb/aerion/internal/contact"
	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/database"
	extauth "github.com/hkdb/aerion/internal/extensions/auth"
	"github.com/hkdb/aerion/internal/extensions/hostaccess"
	"github.com/hkdb/aerion/internal/kit/davutil"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/oauth2"
)

// customOAuthProviderName is the oauth_tokens.provider value used for generic
// OIDC ("bring your own app") accounts. Mirrors app/oauth.go.
const customOAuthProviderName = "custom"

// contactsRuntime is the helper-side contacts stack: the same stores, the
// same Contacts extension backend API and the same admin helpers the desktop
// runs, driven headlessly behind the capability-gated v1 routes.
type contactsRuntime struct {
	contactStore *contact.Store
	carddavStore *carddav.Store
	creds        *credentials.Store
	accounts     *account.Store
	oauth        *oauth2.Manager
	api          *extcontactsbe.API
	extStore     *extcontactsbe.Store
	syncer       *carddav.Syncer
	scheduler    *carddav.Scheduler

	tokenMu        sync.Mutex // serializes standalone source token refresh + source delete/authorize
	accountTokenMu sync.Mutex // serializes account OAuth token refresh
}

func newContactsRuntime(db *database.DB, dataDir string, accounts *account.Store, oauthMgr *oauth2.Manager) (*contactsRuntime, error) {
	if db == nil || db.DB == nil {
		return nil, fmt.Errorf("contacts runtime requires an open database")
	}
	if accounts == nil || oauthMgr == nil {
		return nil, fmt.Errorf("contacts runtime requires the account store and OAuth manager")
	}
	r := &contactsRuntime{
		contactStore: contact.NewStore(db.DB),
		carddavStore: carddav.NewStore(db.DB),
		accounts:     accounts,
		oauth:        oauthMgr,
	}
	creds, err := credentials.NewStore(db.DB, dataDir)
	if err != nil {
		return nil, fmt.Errorf("contacts credentials store: %w", err)
	}
	r.creds = creds

	broker := extauth.NewBroker(creds, oauthMgr, nil)
	auth, err := hostaccess.NewAuth(broker, "contacts", contacts.Manifest(), nil)
	if err != nil {
		return nil, fmt.Errorf("contacts extension auth: %w", err)
	}
	secrets, err := hostaccess.NewSecrets(creds, "contacts")
	if err != nil {
		return nil, fmt.Errorf("contacts extension secrets: %w", err)
	}
	hostSecrets := hostaccess.NewHostSecrets(creds.GetCardDAVPassword)

	r.syncer = carddav.NewSyncer(r.carddavStore, creds)
	r.syncer.SetAccessTokenGetters(r.getAccountAccessToken, r.getStandaloneSourceToken)

	core := newExtensionCore(auth, secrets, hostSecrets, contactsCoreSurface{store: r.carddavStore, syncer: r.syncer})

	extStore, err := extcontactsbe.NewStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("contacts extension store: %w", err)
	}
	r.extStore = extStore
	r.api = extcontactsbe.NewAPI(r.contactStore, r.carddavStore, extStore, core, db.DB)
	r.api.SetStandaloneSourceTokenGetter(r.getStandaloneSourceToken)
	r.scheduler = carddav.NewScheduler(r.syncer, r.carddavStore)
	return r, nil
}

// Close releases the contacts extension store. Stores share the runtime DB,
// which the SyncRuntime closes.
func (r *contactsRuntime) Close() error {
	if r == nil {
		return nil
	}
	if r.scheduler != nil {
		r.scheduler.Stop()
	}
	if r.extStore != nil {
		return r.extStore.Close()
	}
	return nil
}

// getStandaloneSourceToken returns a valid OAuth access token for a
// standalone contact source, refreshing and persisting when it expires soon.
// Mirrors App.getValidContactSourceOAuthToken.
func (r *contactsRuntime) getStandaloneSourceToken(sourceID string) (string, error) {
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	log := logging.WithComponent("alwayson.contacts")

	tokens, err := r.creds.GetContactSourceOAuthTokens(sourceID)
	if err != nil {
		return "", fmt.Errorf("authorize this contact source in Contacts settings: %w", err)
	}
	if tokens.IsExpiringSoon(5 * time.Minute) {
		provider, err := oauth2.ContactRefreshProvider(tokens.Provider)
		if err != nil {
			return "", err
		}
		newTokenResp, err := r.oauth.RefreshTokenWithProvider(provider, tokens.RefreshToken)
		if err != nil {
			log.Error().Err(err).Str("source_id", sourceID).Msg("Contact source OAuth token refresh failed")
			return "", fmt.Errorf("contact source OAuth token refresh failed: %w", err)
		}
		tokens.AccessToken = newTokenResp.AccessToken
		tokens.ExpiresAt = time.Now().Add(time.Duration(newTokenResp.ExpiresIn) * time.Second)
		if newTokenResp.RefreshToken != "" {
			tokens.RefreshToken = newTokenResp.RefreshToken
		}
		if err := r.creds.SetContactSourceOAuthTokens(sourceID, tokens); err != nil {
			return "", fmt.Errorf("persist refreshed contact source token: %w", err)
		}
	}
	return tokens.AccessToken, nil
}

// getAccountAccessToken returns a valid OAuth access token for a mail
// account, refreshing via the custom or named provider when needed. Mirrors
// the helper's scheduled-send token handling.
func (r *contactsRuntime) getAccountAccessToken(accountID string) (string, error) {
	r.accountTokenMu.Lock()
	defer r.accountTokenMu.Unlock()

	tokens, err := r.creds.GetOAuthTokens(accountID)
	if err != nil {
		return "", err
	}
	if !tokens.IsExpiringSoon(5 * time.Minute) {
		return tokens.AccessToken, nil
	}
	var refreshed *oauth2.TokenResponse
	if tokens.Provider == customOAuthProviderName {
		provider, ok, err := r.creds.GetCustomOAuthProvider(accountID)
		if err != nil || !ok {
			return "", fmt.Errorf("custom OAuth configuration unavailable for account %s", accountID)
		}
		config := oauth2.CustomProviderConfig(provider.AuthURL, provider.TokenURL, provider.UserinfoEndpoint, provider.Scopes, provider.ClientID, provider.ClientSecret)
		refreshed, err = r.oauth.RefreshTokenWithProvider(config, tokens.RefreshToken)
		if err != nil {
			return "", err
		}
	}
	if tokens.Provider != customOAuthProviderName {
		refreshed, err = r.oauth.RefreshToken(tokens.Provider, tokens.RefreshToken)
		if err != nil {
			return "", err
		}
	}
	tokens.AccessToken, tokens.RefreshToken = refreshed.AccessToken, refreshed.RefreshToken
	tokens.ExpiresAt = time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	if err := r.creds.SetOAuthTokens(accountID, tokens); err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

// contactsCoreSurface is the scoped coreapi.Contacts surface the helper
// exposes to the contacts backend: source listing plus write/sync state.
// Contact CRUD is served by the backend API itself; interactive linking and
// event subscriptions are desktop-only.
type contactsCoreSurface struct {
	store  *carddav.Store
	syncer *carddav.Syncer
}

func (s contactsCoreSurface) SearchContacts(string, int) ([]coreapi.Contact, error) {
	return nil, coreapi.ErrUnimplemented
}
func (s contactsCoreSurface) GetContact(string) (*coreapi.Contact, error) {
	return nil, coreapi.ErrUnimplemented
}
func (s contactsCoreSurface) ListContacts(coreapi.ContactFilter) ([]coreapi.Contact, error) {
	return nil, coreapi.ErrUnimplemented
}
func (s contactsCoreSurface) ListAddressbooks(string) ([]coreapi.Addressbook, error) {
	return nil, coreapi.ErrUnimplemented
}
func (s contactsCoreSurface) ListSources() ([]coreapi.ContactSource, error) {
	sources, err := s.store.ListSources()
	if err != nil {
		return nil, err
	}
	out := make([]coreapi.ContactSource, 0, len(sources))
	for _, src := range sources {
		if src == nil {
			continue
		}
		cs := coreapi.ContactSource{
			ID: src.ID, Name: src.Name, Type: string(src.Type), Writable: src.Writable,
		}
		if src.AccountID != nil {
			cs.AccountID = *src.AccountID
		}
		out = append(out, cs)
	}
	return out, nil
}
func (s contactsCoreSurface) SetSourceWritable(sourceID string, writable bool) error {
	return s.store.SetSourceWritable(sourceID, writable)
}
func (s contactsCoreSurface) LinkAccountSource(string, string, int) (string, error) {
	return "", fmt.Errorf("%w: link account contact source runs on the interactive desktop", coreapi.ErrUnimplemented)
}
func (s contactsCoreSurface) SyncSource(sourceID string) error {
	return s.syncer.SyncSource(sourceID)
}
func (s contactsCoreSurface) SyncAllSources() error {
	return s.syncer.SyncAllSources()
}
func (s contactsCoreSurface) CreateContact(coreapi.ContactCreateInput) (string, error) {
	return "", fmt.Errorf("%w: contact CRUD is served by the contacts backend API on this host", coreapi.ErrUnimplemented)
}
func (s contactsCoreSurface) UpdateContact(string, coreapi.ContactPatch) error {
	return fmt.Errorf("%w: contact CRUD is served by the contacts backend API on this host", coreapi.ErrUnimplemented)
}
func (s contactsCoreSurface) DeleteContact(string) error {
	return fmt.Errorf("%w: contact CRUD is served by the contacts backend API on this host", coreapi.ErrUnimplemented)
}
func (s contactsCoreSurface) SubscribeToContactEvents([]coreapi.ContactEventType) (<-chan coreapi.ContactEvent, coreapi.Unsubscribe, error) {
	return nil, nil, fmt.Errorf("%w: contact event subscriptions are desktop-only", coreapi.ErrUnimplemented)
}

var _ coreapi.Contacts = contactsCoreSurface{}

// contactsOperations adapts contactsRuntime to the v1.ContactsOperations
// surface mounted on the helper's automation API.
type contactsOperations struct {
	runtime *contactsRuntime
}

func legacyContactResult(c *contact.Contact) v1.ContactSearchResult {
	return v1.ContactSearchResult{
		ID: c.Email, Email: c.Email, DisplayName: c.DisplayName, Source: c.Source,
		Kind: c.Kind, AvatarURL: c.AvatarURL, SendCount: c.SendCount,
		LastUsed: c.LastUsed, CreatedAt: c.CreatedAt,
	}
}

func (o *contactsOperations) SearchContacts(query string, limit int) ([]v1.ContactSearchResult, error) {
	// Synced Google contacts, saved/collected addresses and vCards are local
	// to the helper's database. Autocomplete must not authorize Contacts or
	// refresh Mail credentials.
	results, err := o.runtime.contactStore.Search(query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]v1.ContactSearchResult, 0, len(results))
	for _, c := range results {
		if c == nil {
			continue
		}
		out = append(out, legacyContactResult(c))
	}
	return out, nil
}

func (o *contactsOperations) ListLegacyContacts(limit int) ([]v1.ContactSearchResult, error) {
	results, err := o.runtime.contactStore.List(limit)
	if err != nil {
		return nil, err
	}
	out := make([]v1.ContactSearchResult, 0, len(results))
	for _, c := range results {
		if c == nil {
			continue
		}
		out = append(out, legacyContactResult(c))
	}
	return out, nil
}

func (o *contactsOperations) GetLegacyContact(id string) (*v1.ContactSearchResult, error) {
	c, err := o.runtime.contactStore.Get(id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, v1.ErrContactNotFound
	}
	result := legacyContactResult(c)
	return &result, nil
}

func (o *contactsOperations) GetContactPhotos(emails []string) ([]contact.ContactPhoto, error) {
	if len(emails) == 0 {
		return []contact.ContactPhoto{}, nil
	}
	return o.runtime.contactStore.GetPhotosByEmails(emails)
}

// GetAccountProfilePhotos returns profile photos for authenticated Google
// mail accounts. Mirrors App.GetAccountProfilePhotos.
func (o *contactsOperations) GetAccountProfilePhotos(emails []string) ([]contact.ContactPhoto, error) {
	log := logging.WithComponent("alwayson.contacts.account-profile-photo")
	wanted := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		if normalized := strings.ToLower(strings.TrimSpace(email)); normalized != "" {
			wanted[normalized] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return []contact.ContactPhoto{}, nil
	}
	accounts, err := o.runtime.accounts.List()
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	photos := make([]contact.ContactPhoto, 0, len(wanted))
	seen := make(map[string]struct{}, len(wanted))
	for _, acc := range accounts {
		email := strings.ToLower(strings.TrimSpace(acc.Email))
		if _, ok := wanted[email]; !ok {
			continue
		}
		isGoogleOAuth := acc.AuthType == account.AuthOAuth2 && strings.Contains(strings.ToLower(acc.IMAPHost), "gmail")
		if _, alreadyAdded := seen[email]; alreadyAdded || !isGoogleOAuth {
			continue
		}
		accessToken, err := o.runtime.getAccountAccessToken(acc.ID)
		if err != nil || accessToken == "" {
			log.Debug().Str("account_id", acc.ID).Str("email", logging.RedactEmail(email)).Str("reason", "no-valid-token").Msg("Account profile photo lookup failed")
			continue
		}
		profileReq, err := http.NewRequest(http.MethodGet, "https://people.googleapis.com/v1/people/me?personFields=photos", nil)
		if err != nil {
			continue
		}
		profileReq.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := client.Do(profileReq)
		if err != nil {
			continue
		}
		var profile struct {
			Photos []struct {
				URL     string `json:"url"`
				Default bool   `json:"default"`
			} `json:"photos"`
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile)
		statusCode := resp.StatusCode
		resp.Body.Close()
		if statusCode != http.StatusOK || decodeErr != nil {
			log.Debug().Str("account_id", acc.ID).Str("email", logging.RedactEmail(email)).Int("status", statusCode).Msg("Account profile photo lookup failed")
			continue
		}
		for _, profilePhoto := range profile.Photos {
			if profilePhoto.Default || profilePhoto.URL == "" {
				continue
			}
			photoReq, err := http.NewRequest(http.MethodGet, profilePhoto.URL, nil)
			if err != nil {
				continue
			}
			photoReq.Header.Set("Authorization", "Bearer "+accessToken)
			photoResp, err := client.Do(photoReq)
			if err != nil {
				continue
			}
			photoData, readErr := io.ReadAll(io.LimitReader(photoResp.Body, 5<<20))
			mediaType := photoResp.Header.Get("Content-Type")
			photoResp.Body.Close()
			if readErr != nil || photoResp.StatusCode != http.StatusOK || len(photoData) == 0 {
				continue
			}
			photos = append(photos, contact.ContactPhoto{Email: email, Data: base64.StdEncoding.EncodeToString(photoData), MediaType: mediaType})
			seen[email] = struct{}{}
			break
		}
	}
	return photos, nil
}

func (o *contactsOperations) BrowseContacts(query, sourceID string, limit, offset int) ([]coreapi.Contact, error) {
	return o.runtime.api.ListContacts(coreapi.ContactFilter{
		Query: query, SourceID: sourceID, Limit: limit, Offset: offset,
	})
}

func (o *contactsOperations) GetContactDetail(id string) (*coreapi.Contact, error) {
	detail, err := o.runtime.api.GetContact(id)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, v1.ErrContactNotFound
	}
	return detail, nil
}

func (o *contactsOperations) ListContactSources() ([]v1.ContactsSourceInfo, error) {
	sources, err := o.runtime.carddavStore.ListSources()
	if err != nil {
		return nil, err
	}
	out := make([]v1.ContactsSourceInfo, 0, len(sources))
	for _, s := range sources {
		if s == nil {
			continue
		}
		s.HasTokens = o.runtime.creds.HasContactSourceOAuthTokens(s.ID)
		out = append(out, *s)
	}
	return out, nil
}

func (o *contactsOperations) GetContactStats() (*v1.ContactsStats, error) {
	sources, err := o.runtime.carddavStore.ListSources()
	if err != nil {
		return nil, err
	}
	totalContacts, _ := o.runtime.carddavStore.CountContacts()
	sourcesWithErrors, _ := o.runtime.carddavStore.GetSourcesWithErrors()
	return &v1.ContactsStats{
		TotalSources: len(sources), TotalContacts: totalContacts, SourcesWithErrors: len(sourcesWithErrors),
	}, nil
}

func (o *contactsOperations) ListCoreAddressbooks(sourceID string) ([]v1.ContactsAddressbookInfo, error) {
	return o.runtime.api.ListAddressbooks(sourceID)
}

func (o *contactsOperations) ListSourceAddressbooks(sourceID string) ([]v1.ContactsSourceAddressbook, error) {
	books, err := o.runtime.carddavStore.ListAddressbooks(sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]v1.ContactsSourceAddressbook, 0, len(books))
	for _, b := range books {
		if b == nil {
			continue
		}
		out = append(out, *b)
	}
	return out, nil
}

func (o *contactsOperations) ListLinkedAccounts(kind string) ([]v1.ContactsLinkedAccount, error) {
	accounts, err := o.runtime.accounts.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	sources, err := o.runtime.carddavStore.ListSources()
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}
	var result []v1.ContactsLinkedAccount
	for _, acc := range accounts {
		if acc.AuthType != account.AuthOAuth2 {
			continue
		}
		provider, err := o.runtime.creds.GetOAuthProvider(acc.ID)
		if err != nil || provider == "" {
			continue
		}
		switch kind {
		case "google":
			// Only Google supports linked contact sync. Microsoft can't share
			// tokens between Outlook (email) and Graph API (contacts).
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
			result = append(result, v1.ContactsLinkedAccount{
				AccountID: acc.ID, Email: acc.Email, Name: acc.Name, Provider: provider,
				IsLinked: contactSourceID != "", ContactSourceID: contactSourceID,
			})
		case "custom-oauth":
			if provider != customOAuthProviderName {
				continue
			}
			linked := false
			for _, source := range sources {
				if source.AccountID != nil && *source.AccountID == acc.ID {
					linked = true
					break
				}
			}
			result = append(result, v1.ContactsLinkedAccount{
				AccountID: acc.ID, Email: acc.Email, Name: acc.Name, Provider: provider, IsLinked: linked,
			})
		}
	}
	if result == nil {
		result = []v1.ContactsLinkedAccount{}
	}
	return result, nil
}

func (o *contactsOperations) MutateContact(req v1.ContactsMutationRequest) (*v1.ContactsMutationResponse, error) {
	rt := o.runtime
	switch req.Operation {
	case "add_legacy":
		if strings.TrimSpace(req.Email) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "An email address is required"}
		}
		return &v1.ContactsMutationResponse{}, rt.contactStore.AddOrUpdate(req.Email, req.DisplayName)
	case "delete_legacy":
		if strings.TrimSpace(req.ID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A contact ID is required"}
		}
		return &v1.ContactsMutationResponse{}, rt.contactStore.Delete(req.ID)
	case "create":
		if req.Create == nil {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A create payload is required"}
		}
		id, err := rt.api.CreateContact(*req.Create)
		if err != nil {
			return nil, err
		}
		return &v1.ContactsMutationResponse{ID: id}, nil
	case "update":
		if strings.TrimSpace(req.ID) == "" || req.Patch == nil {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A contact ID and patch are required"}
		}
		return &v1.ContactsMutationResponse{}, rt.api.UpdateContact(req.ID, *req.Patch)
	case "delete":
		if strings.TrimSpace(req.ID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A contact ID is required"}
		}
		return &v1.ContactsMutationResponse{}, rt.api.DeleteContact(req.ID)
	case "set_writable":
		if strings.TrimSpace(req.ID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A source ID is required"}
		}
		if req.Writable {
			source, err := rt.carddavStore.GetSource(req.ID)
			if err != nil {
				return nil, err
			}
			if source == nil {
				return nil, v1.ErrContactNotFound
			}
			if source.Type == carddav.SourceTypeGoogle {
				return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "Google Contacts is read-only"}
			}
		}
		return &v1.ContactsMutationResponse{}, rt.carddavStore.SetSourceWritable(req.ID, req.Writable)
	case "set_addressbook_enabled":
		if strings.TrimSpace(req.AddressbookID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "An addressbook ID is required"}
		}
		return &v1.ContactsMutationResponse{}, rt.carddavStore.SetAddressbookEnabled(req.AddressbookID, req.Enabled)
	case "sync":
		if strings.TrimSpace(req.ID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A source ID is required"}
		}
		return &v1.ContactsMutationResponse{}, rt.syncer.SyncSource(req.ID)
	case "sync_all":
		return &v1.ContactsMutationResponse{}, rt.syncer.SyncAllSources()
	case "force_sync":
		if strings.TrimSpace(req.ID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A source ID is required"}
		}
		if err := carddav.ForceSyncAdmin(rt.carddavStore, req.ID); err != nil {
			return nil, err
		}
		return &v1.ContactsMutationResponse{}, rt.syncer.SyncSource(req.ID)
	}
	return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "Unknown contact mutation operation"}
}

func (o *contactsOperations) DiscoverAddressbooks(req v1.ContactsDiscoverRequest) ([]v1.ContactsDiscoveredAddressbook, error) {
	if req.AccountID != "" {
		// Unified-grant path: a custom OIDC account whose token also
		// authorizes CardDAV (e.g. Stalwart). The account token getter is
		// custom-OAuth-aware and refreshing. The bearer token must never
		// leave the account's own mail server: validate the target before
		// dialing so a contacts.manage client cannot exfiltrate tokens to
		// an arbitrary (or plain-HTTP) host.
		acc, err := o.runtime.accounts.Get(req.AccountID)
		if errors.Is(err, account.ErrAccountNotFound) {
			return nil, v1.ErrContactNotFound
		}
		if err != nil {
			return nil, err
		}
		if acc == nil {
			return nil, v1.ErrContactNotFound
		}
		if err := validateContactsDiscoveryURL(req.URL, acc); err != nil {
			return nil, err
		}
		accessToken, err := o.runtime.getAccountAccessToken(req.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get OAuth token: %w", err)
		}
		httpClient := davutil.NewBearerHTTPClient(accessToken, 30*time.Second)
		return carddav.DiscoverAddressbooksWithHTTPClient(req.URL, httpClient)
	}
	return carddav.DiscoverAddressbooks(req.URL, req.Username, req.Password)
}

// validateContactsDiscoveryURL constrains bearer-token discovery to HTTPS
// URLs on the account's own IMAP/SMTP host (loopback exempt for local
// servers, still the account's host). Unified-grant servers such as
// Stalwart serve CardDAV from the same host as mail.
func validateContactsDiscoveryURL(rawURL string, acc *account.Account) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A valid discovery URL is required"}
	}
	host := strings.ToLower(u.Hostname())
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if u.Scheme != "https" && !loopback {
		return &v1.Fault{Code: v1.CodeInvalidRequest, Message: "OAuth addressbook discovery requires an HTTPS URL"}
	}
	matchesAccountHost := func(accountHost string) bool {
		accountHost = strings.ToLower(strings.TrimSpace(accountHost))
		return accountHost != "" && (host == accountHost || strings.HasSuffix(host, "."+accountHost))
	}
	if !matchesAccountHost(acc.IMAPHost) && !matchesAccountHost(acc.SMTPHost) {
		return &v1.Fault{Code: v1.CodeInvalidRequest, Message: "OAuth discovery URL must match the account's mail server host"}
	}
	return nil
}

func (o *contactsOperations) TestConnection(req v1.ContactsDiscoverRequest) error {
	return carddav.TestConnection(req.URL, req.Username, req.Password)
}

func (o *contactsOperations) MutateSource(req v1.ContactsSourceMutationRequest) (*v1.ContactsSourceInfo, error) {
	rt := o.runtime
	log := logging.WithComponent("alwayson.contacts")
	switch req.Operation {
	case "create":
		source, err := carddav.CreateSourceAdmin(rt.carddavStore, rt.creds, *req.Config)
		if err != nil {
			return nil, err
		}
		go func(id string) {
			if err := rt.syncer.SyncSource(id); err != nil {
				log.Warn().Err(err).Str("source_id", id).Msg("Background contact source sync failed")
			}
		}(source.ID)
		return source, nil
	case "update":
		if strings.TrimSpace(req.ID) == "" {
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A source ID is required"}
		}
		if err := carddav.UpdateSourceAdmin(rt.carddavStore, rt.creds, req.ID, *req.Config); err != nil {
			return nil, err
		}
		go func(id string) {
			if err := rt.syncer.SyncSource(id); err != nil {
				log.Warn().Err(err).Str("source_id", id).Msg("Background contact source sync failed")
			}
		}(req.ID)
		source, err := rt.carddavStore.GetSource(req.ID)
		if err != nil {
			return nil, err
		}
		if source == nil {
			return nil, v1.ErrContactNotFound
		}
		return source, nil
	case "delete":
		rt.tokenMu.Lock()
		defer rt.tokenMu.Unlock()
		if err := carddav.DeleteSourceAdmin(rt.carddavStore, rt.creds, req.ID); err != nil {
			return nil, err
		}
		return &v1.ContactsSourceInfo{ID: req.ID}, nil
	}
	return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: "Unknown source mutation operation"}
}

func (o *contactsOperations) SaveSourceAuthorization(req v1.ContactsAuthorizationRequest) (*v1.ContactsSourceInfo, error) {
	rt := o.runtime
	provider, err := oauth2.ContactRefreshProvider(req.Provider)
	if err != nil {
		return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: err.Error()}
	}
	source, err := carddav.SaveAuthorization(rt.carddavStore, rt.creds, &rt.tokenMu, carddav.AuthorizationParams{
		SourceID: req.SourceID, AccountID: req.AccountID, Name: req.Name,
		SyncInterval: req.SyncInterval, Provider: provider.Name, Email: req.Email,
		AccessToken: req.Tokens.AccessToken, RefreshToken: req.Tokens.RefreshToken,
		ExpiresIn: req.Tokens.ExpiresIn, Scopes: provider.Scopes,
	})
	if err != nil {
		// Keep the user-actionable messages; everything else stays generic.
		switch msg := err.Error(); {
		case strings.Contains(msg, "already exists; reauthorize"):
			return nil, &v1.Fault{Code: v1.CodeConflict, Message: msg}
		case strings.Contains(msg, "no longer exists"):
			return nil, v1.ErrContactNotFound
		case strings.Contains(msg, "no refresh token"):
			return nil, &v1.Fault{Code: v1.CodeInvalidRequest, Message: msg}
		}
		return nil, err
	}
	go func(id string) {
		if err := rt.syncer.SyncSource(id); err != nil {
			log := logging.WithComponent("alwayson.contacts")
			log.Warn().Err(err).Str("source_id", id).Msg("Background contact source sync failed")
		}
	}(source.ID)
	return source, nil
}

func (o *contactsOperations) StoreSourceTokens(req v1.ContactsSourceTokensRequest) error {
	rt := o.runtime
	rt.tokenMu.Lock()
	defer rt.tokenMu.Unlock()
	expiresAt := time.Now().Add(time.Duration(req.Tokens.ExpiresIn) * time.Second)
	if req.SourceID != "" {
		source, err := rt.carddavStore.GetSource(req.SourceID)
		if err != nil {
			return err
		}
		if source == nil {
			return v1.ErrContactNotFound
		}
		tokens := &credentials.OAuthTokens{
			Provider: req.Provider, AccessToken: req.Tokens.AccessToken,
			RefreshToken: req.Tokens.RefreshToken, ExpiresAt: expiresAt, Scopes: req.Scopes,
		}
		if tokens.RefreshToken == "" {
			if previous, err := rt.creds.GetContactSourceOAuthTokens(req.SourceID); err == nil && previous.Provider == tokens.Provider {
				tokens.RefreshToken = previous.RefreshToken
			}
		}
		if tokens.RefreshToken == "" {
			return &v1.Fault{Code: v1.CodeInvalidRequest, Message: "A refresh token is required"}
		}
		return rt.creds.SetContactSourceOAuthTokens(req.SourceID, tokens)
	}
	// Account-keyed persistence, scoped to the client config the consent ran
	// under (incremental-consent path). A contacts.manage grant must never
	// reach another surface's credential slot, so the target config must be
	// the provider's contacts config (never e.g. google-mail) and the account
	// must exist.
	if req.Provider != "google" && req.Provider != "microsoft" {
		return &v1.Fault{Code: v1.CodeInvalidRequest, Message: "Unsupported contacts OAuth provider"}
	}
	if req.ClientConfigID != req.Provider+"-contacts" {
		return &v1.Fault{Code: v1.CodeInvalidRequest, Message: "Source tokens must target the provider's contacts client configuration"}
	}
	acc, err := rt.accounts.Get(req.AccountID)
	if errors.Is(err, account.ErrAccountNotFound) {
		return v1.ErrContactNotFound
	}
	if err != nil {
		return err
	}
	if acc == nil {
		return v1.ErrContactNotFound
	}
	tokens := &credentials.OAuthTokens{
		Provider: req.Provider, AccessToken: req.Tokens.AccessToken,
		RefreshToken: req.Tokens.RefreshToken, ExpiresAt: expiresAt, Scopes: req.Scopes,
	}
	return rt.creds.SetOAuthTokensForClientConfig(req.AccountID, req.ClientConfigID, tokens)
}
