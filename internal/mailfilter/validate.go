package mailfilter

import (
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
)

// ValidateRule checks a rule's structure against the account and folder
// catalogs owned by the caller: the rule must have a name, at least one
// action, at most one relocation action, an existing destination folder when
// it moves, and existing accounts for every scoped account. Same-account
// destinations are allowed.
func ValidateRule(rule *Rule, accounts interface {
	Get(string) (*account.Account, error)
}, folders interface {
	Get(string) (*folder.Folder, error)
}) error {
	if rule == nil {
		return fmt.Errorf("filter rule cannot be nil")
	}
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("filter rule name cannot be empty")
	}
	if !rule.HasActions() {
		return fmt.Errorf("filter rule needs at least one action")
	}
	if rule.RelocationCount() > 1 {
		return fmt.Errorf("filter rule can have only one of move to folder, archive, or delete to Trash")
	}
	if rule.DestFolderID != "" {
		dest, err := folders.Get(rule.DestFolderID)
		if err != nil {
			return fmt.Errorf("failed to load destination folder: %w", err)
		}
		if dest == nil {
			return fmt.Errorf("destination folder not found: %s", rule.DestFolderID)
		}
	}
	for _, id := range rule.AccountIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("selected account ID cannot be empty")
		}
	}
	for _, accountID := range rule.ScopeAccountIDs() {
		acct, err := accounts.Get(accountID)
		if err != nil {
			return fmt.Errorf("failed to load source account: %w", err)
		}
		if acct == nil {
			return fmt.Errorf("source account not found: %s", accountID)
		}
	}
	return nil
}
