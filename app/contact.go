package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/contact"
	"github.com/hkdb/aerion/internal/logging"
)

// ============================================================================
// Contact API - Exposed to frontend via Wails bindings
// ============================================================================

func (a *App) requireLocalContacts() error {
	if a.remoteServiceMode {
		return errors.New("Contacts and CardDAV are unavailable in remote mailbox mode")
	}
	return nil
}

// SearchContacts searches for contacts matching the query
// Returns contacts from multiple sources: local database, vCard files, CardDAV, and Google Contacts
func (a *App) SearchContacts(query string, limit int) ([]*contact.Contact, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		results, err := client.SearchContacts(a.ctx, query, limit)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		out := make([]*contact.Contact, 0, len(results))
		for i := range results {
			r := results[i]
			out = append(out, &contact.Contact{Email: r.Email, DisplayName: r.DisplayName, Source: r.Source, Kind: r.Kind, AvatarURL: r.AvatarURL, SendCount: r.SendCount, LastUsed: r.LastUsed, CreatedAt: r.CreatedAt})
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	// Synced Google contacts, saved/collected addresses and vCards are local.
	// Autocomplete must not authorize Contacts or refresh Mail credentials.
	return a.contactStore.Search(query, limit)
}

// GetContact returns a single contact by ID
func (a *App) GetContact(id string) (*contact.Contact, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		r, err := client.GetLegacyContact(a.ctx, id)
		if err != nil {
			// Local GetContact answers a missing contact with (nil, nil);
			// callers treat it as a normal lookup result, not a failure.
			if v1.ErrorCode(remoteContactsFaultCode(err)) == v1.CodeNotFound {
				return nil, nil
			}
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		if r == nil {
			return nil, nil
		}
		return &contact.Contact{Email: r.Email, DisplayName: r.DisplayName, Source: r.Source, Kind: r.Kind, AvatarURL: r.AvatarURL, SendCount: r.SendCount, LastUsed: r.LastUsed, CreatedAt: r.CreatedAt}, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.contactStore.Get(id)
}

// AddContact adds or updates a contact
func (a *App) AddContact(email, displayName string) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		req := v1.ContactsMutationRequest{Operation: "add_legacy", Email: email, DisplayName: displayName}
		body := contactsCanonicalBody(req)
		key, err := contactsIdempotencyKey("mutate", body)
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, req, key)
		settleContactsMutation(err, "mutate", body)
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.contactStore.AddOrUpdate(email, displayName)
}

// DeleteContact deletes a contact
func (a *App) DeleteContact(id string) error {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return err
		}
		req := v1.ContactsMutationRequest{Operation: "delete_legacy", ID: id}
		body := contactsCanonicalBody(req)
		key, err := contactsIdempotencyKey("mutate", body)
		if err != nil {
			return err
		}
		_, err = client.MutateContact(a.ctx, req, key)
		settleContactsMutation(err, "mutate", body)
		return remoteContactsError(err, v1.CapabilityContactsWrite)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	return a.contactStore.Delete(id)
}

// ListContacts returns all contacts
func (a *App) ListContacts(limit int) ([]*contact.Contact, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		results, err := client.ListLegacyContacts(a.ctx, limit)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		out := make([]*contact.Contact, 0, len(results))
		for i := range results {
			r := results[i]
			out = append(out, &contact.Contact{Email: r.Email, DisplayName: r.DisplayName, Source: r.Source, Kind: r.Kind, AvatarURL: r.AvatarURL, SendCount: r.SendCount, LastUsed: r.LastUsed, CreatedAt: r.CreatedAt})
		}
		return out, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	return a.contactStore.List(limit)
}

// GetContactPhotos returns inline contact photos for the given emails, keyed by
// lowercased email. Used by the message list to render contact profile pictures
// in the avatar slot (opt-in setting). Resolves the whole batch in one query —
// the frontend batches per list load rather than calling per row.
func (a *App) GetContactPhotos(emails []string) ([]contact.ContactPhoto, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		photos, err := client.GetContactPhotos(a.ctx, emails)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		return photos, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	if len(emails) == 0 {
		return []contact.ContactPhoto{}, nil
	}
	return a.contactStore.GetPhotosByEmails(emails)
}

// GetAccountProfilePhotos returns profile photos for authenticated Google mail
// accounts. A Google account's own profile is not part of People connections,
// so it cannot be obtained through GetContactPhotos.
func (a *App) GetAccountProfilePhotos(emails []string) ([]contact.ContactPhoto, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		photos, err := client.GetAccountProfilePhotos(a.ctx, emails)
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsRead)
		}
		return photos, nil
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	log := logging.WithComponent("app.account-profile-photo")

	wanted := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		if normalized := strings.ToLower(strings.TrimSpace(email)); normalized != "" {
			wanted[normalized] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return []contact.ContactPhoto{}, nil
	}

	accounts, err := a.accountStore.List()
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

		tokens, err := a.getValidOAuthToken(acc.ID)
		if err != nil || tokens == nil || tokens.AccessToken == "" {
			log.Debug().Str("account_id", acc.ID).Str("email", logging.RedactEmail(email)).Str("reason", "no-valid-token").Msg("Account profile photo lookup failed")
			continue
		}

		profileReq, err := http.NewRequest(http.MethodGet, "https://people.googleapis.com/v1/people/me?personFields=photos", nil)
		if err != nil {
			continue
		}
		profileReq.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
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
		if statusCode != http.StatusOK {
			log.Debug().
				Str("account_id", acc.ID).
				Str("email", logging.RedactEmail(email)).
				Int("status", statusCode).
				Int("google_error_code", profile.Error.Code).
				Msg("Account profile photo lookup failed")
		} else if decodeErr != nil {
			log.Debug().Str("account_id", acc.ID).Str("email", logging.RedactEmail(email)).Int("status", statusCode).Str("reason", "invalid-response").Msg("Account profile photo lookup failed")
		}
		if statusCode != http.StatusOK || decodeErr != nil {
			continue
		}

		for _, profilePhoto := range profile.Photos {
			if profilePhoto.Default || profilePhoto.URL == "" {
				continue
			}
			data, mediaType, ok := contact.FetchInlinePhotoURL(client, profilePhoto.URL)
			if !ok {
				continue
			}
			photos = append(photos, contact.ContactPhoto{Email: email, Data: data, MediaType: mediaType})
			seen[email] = struct{}{}
			break
		}
	}

	return photos, nil
}
