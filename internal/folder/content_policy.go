package folder

import (
	"database/sql"
	"fmt"

	"github.com/hkdb/aerion/internal/database"
)

// ContentPolicyOverride is an explicit account-qualified folder content choice.
// A missing row means the effective value should be derived from sidebar use.
type ContentPolicyOverride struct {
	AccountID string `json:"accountId"`
	FolderID  string `json:"folderId"`
	Enabled   bool   `json:"enabled"`
}

// ContentPolicyStore persists explicit body-prefetch choices. It deliberately
// does not store message content or attachment data.
type ContentPolicyStore struct {
	db *database.DB
}

// NewContentPolicyStore creates a store for folder body-prefetch overrides.
func NewContentPolicyStore(db *database.DB) *ContentPolicyStore {
	return &ContentPolicyStore{db: db}
}

// Get returns an explicit override, or nil when the folder inherits its default.
func (s *ContentPolicyStore) Get(accountID, folderID string) (*ContentPolicyOverride, error) {
	if accountID == "" || folderID == "" {
		return nil, fmt.Errorf("account and folder IDs are required")
	}
	var enabled bool
	err := s.db.QueryRow(`
		SELECT body_prefetch FROM folder_content_policies
		WHERE account_id = ? AND folder_id = ?`, accountID, folderID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get folder content policy: %w", err)
	}
	return &ContentPolicyOverride{AccountID: accountID, FolderID: folderID, Enabled: enabled}, nil
}

// Set writes an explicit on/off choice for a folder.
func (s *ContentPolicyStore) Set(accountID, folderID string, enabled bool) error {
	if accountID == "" || folderID == "" {
		return fmt.Errorf("account and folder IDs are required")
	}
	_, err := s.db.Exec(`
		INSERT INTO folder_content_policies(folder_id, account_id, body_prefetch)
		VALUES(?, ?, ?)
		ON CONFLICT(folder_id) DO UPDATE SET
			account_id = excluded.account_id,
			body_prefetch = excluded.body_prefetch,
			updated_at = datetime('now')`, folderID, accountID, enabled)
	if err != nil {
		return fmt.Errorf("set folder content policy: %w", err)
	}
	return nil
}

// Reset removes an explicit choice so sidebar-derived defaults apply again.
func (s *ContentPolicyStore) Reset(accountID, folderID string) error {
	if accountID == "" || folderID == "" {
		return fmt.Errorf("account and folder IDs are required")
	}
	_, err := s.db.Exec(`DELETE FROM folder_content_policies WHERE account_id = ? AND folder_id = ?`, accountID, folderID)
	if err != nil {
		return fmt.Errorf("reset folder content policy: %w", err)
	}
	return nil
}
