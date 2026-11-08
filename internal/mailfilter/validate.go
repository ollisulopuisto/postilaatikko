package mailfilter

import (
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
)

// ValidateRule checks the cross-account destination invariant against the
// account and folder catalogs owned by the caller.
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
	if rule.DestFolderID == "" {
		return fmt.Errorf("filter rule destination folder is required")
	}
	dest, err := folders.Get(rule.DestFolderID)
	if err != nil {
		return fmt.Errorf("failed to load destination folder: %w", err)
	}
	if dest == nil {
		return fmt.Errorf("destination folder not found: %s", rule.DestFolderID)
	}
	if rule.AccountID == "" {
		return nil
	}
	acct, err := accounts.Get(rule.AccountID)
	if err != nil {
		return fmt.Errorf("failed to load source account: %w", err)
	}
	if acct == nil {
		return fmt.Errorf("source account not found: %s", rule.AccountID)
	}
	if dest.AccountID == rule.AccountID {
		return fmt.Errorf("destination folder must be on a different account than the rule's source account")
	}
	return nil
}
