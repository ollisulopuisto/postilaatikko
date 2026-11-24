// Package accountsetup provides offline local account administration. It never
// starts sync or sends mail and refuses a profile open by another process.
package accountsetup

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	"github.com/ollisulopuisto/postilaatikko/internal/syncowner"
)

type Store struct {
	db       *database.DB
	accounts *account.Store
	creds    *credentials.Store
	lock     *syncowner.Lock
}

// Open holds the exclusive schema lock for the complete setup session. Stop the
// desktop and helper before use. Existing schema migrations stay owner-locked.
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("account setup requires a data directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := syncowner.TryAcquire(filepath.Join(dir, "mail-schema.owner.lock"))
	if err != nil {
		return nil, errors.New("stop Postilaatikko and its helper before adding an account")
	}
	db, err := database.Open(filepath.Join(dir, "aerion.db"))
	if err != nil {
		lock.Release()
		return nil, err
	}
	fail := func(err error) (*Store, error) { db.Close(); lock.Release(); return nil, err }
	if current, err := db.CurrentSchemaVersion(); err != nil {
		return fail(err)
	} else if current > database.MaxSchemaVersion() {
		return fail(errors.New("profile schema is newer than this helper"))
	}
	if err = db.Migrate(); err != nil {
		return fail(err)
	}
	creds, err := credentials.NewStore(db.DB, dir)
	if err != nil {
		return fail(err)
	}
	return &Store{db: db, accounts: account.NewStore(db), creds: creds, lock: lock}, nil
}

func (s *Store) Close() error { return errors.Join(s.db.Close(), s.lock.Release()) }

// Create accepts credentials only in memory. On failed persistence it removes
// secrets before deleting the new account, retaining a disabled repair record
// if cleanup fails. It never overwrites an existing account.
func (s *Store) Create(config account.AccountConfig, password, smtpPassword string, tokens *credentials.OAuthTokens) (*account.Account, error) {
	parsed, err := mail.ParseAddress(config.Email)
	if err != nil || parsed.Address != config.Email {
		return nil, errors.New("a plain email address is required")
	}
	if config.AuthType == account.AuthOAuth2 {
		if tokens == nil || tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.ExpiresAt.IsZero() || (tokens.Provider != "google" && tokens.Provider != "microsoft") {
			return nil, errors.New("complete OAuth consent with offline access before creating an account")
		}
	} else if password == "" {
		return nil, errors.New("incoming password is required")
	}
	// Case-insensitive duplicate guard while the exclusive profile lock is held.
	existing, err := s.accounts.List()
	if err != nil {
		return nil, err
	}
	for _, a := range existing {
		if strings.EqualFold(a.Email, config.Email) {
			return nil, account.ErrAccountExists
		}
	}
	acc, err := s.accounts.Create(&config)
	if err != nil {
		// The legacy writer inserts its identity separately. Clean up a partial
		// new row on identity failure; the exclusive lock and duplicate guard
		// ensure this cannot remove an account that preceded this operation.
		rows, listErr := s.accounts.List()
		if listErr != nil {
			return nil, errors.New("account creation failed; inspect profile before retrying")
		}
		for _, row := range rows {
			if strings.EqualFold(row.Email, config.Email) {
				if cleanupErr := s.accounts.Delete(row.ID); cleanupErr != nil {
					_ = s.accounts.SetEnabled(row.ID, false)
					return nil, fmt.Errorf("account %s retained disabled for repair", row.ID)
				}
			}
		}
		return nil, err
	}
	fail := func() (*account.Account, error) {
		if err := s.creds.DeleteAllCredentials(acc.ID); err != nil {
			_ = s.accounts.SetEnabled(acc.ID, false)
			return nil, fmt.Errorf("account %s retained disabled for credential repair", acc.ID)
		}
		if err := s.accounts.Delete(acc.ID); err != nil {
			_ = s.accounts.SetEnabled(acc.ID, false)
			return nil, fmt.Errorf("account %s retained disabled for repair", acc.ID)
		}
		return nil, errors.New("account credentials could not be saved; account creation rolled back")
	}
	if tokens != nil {
		if err = s.creds.SetOAuthTokens(acc.ID, tokens); err != nil {
			return fail()
		}
	} else {
		if err = s.creds.SetPassword(acc.ID, password); err != nil {
			return fail()
		}
		if smtpPassword != "" {
			if err = s.creds.SetSMTPPassword(acc.ID, smtpPassword); err != nil {
				return fail()
			}
		}
	}
	return acc, nil
}

// Provider resolves this profile's saved desktop-client overrides without
// changing process-global resolver hooks.
func (s *Store) Provider(name string) (oauth2.ProviderConfig, error) {
	cfg, err := oauth2.GetProvider(name)
	if err != nil {
		return cfg, err
	}
	slot := oauth2.ClientConfigIDForProvider(name)
	choice, err := s.creds.GetOAuthActiveChoice(slot)
	if err != nil {
		return cfg, err
	}
	if choice == "aerion-shipped" {
		return cfg, nil
	}
	if choice == "" || choice == "custom" {
		if id, secret, ok, err := s.creds.GetUserClientCreds(slot); err != nil {
			return cfg, err
		} else if ok {
			cfg.ClientID, cfg.ClientSecret = id, secret
			return cfg, nil
		}
		if choice == "custom" {
			return cfg, nil
		}
	}
	if target, ok, err := s.creds.GetOAuthSlotAlias(slot); err != nil {
		return cfg, err
	} else if ok && target != slot {
		if id, secret, ok, err := s.creds.GetUserClientCreds(target); err != nil {
			return cfg, err
		} else if ok {
			cfg.ClientID, cfg.ClientSecret = id, secret
		} else if shipped, ok := oauth2.ShippedClientConfigForID(target); ok {
			cfg.ClientID, cfg.ClientSecret = shipped.ClientID, shipped.ClientSecret
		}
	}
	return cfg, nil
}
