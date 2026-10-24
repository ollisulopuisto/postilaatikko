package app

import (
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/message"
)

// ============================================================================
// Local Cross-Account Filter API - Exposed to frontend via Wails bindings
// ============================================================================

// GetFilterRules returns all local cross-account filter rules in priority order.
func (a *App) GetFilterRules() ([]*mailfilter.Rule, error) {
	return a.filterStore.List()
}

// CreateFilterRule validates and persists a new local cross-account filter rule.
func (a *App) CreateFilterRule(rule *mailfilter.Rule) error {
	if rule == nil {
		return fmt.Errorf("filter rule cannot be nil")
	}
	if err := a.validateFilterRule(rule); err != nil {
		return err
	}
	// New rules go to the end of the priority list.
	if rule.OrderIndex == 0 {
		if existing, listErr := a.filterStore.List(); listErr == nil {
			rule.OrderIndex = len(existing)
		}
	}
	return a.filterStore.Create(rule)
}

// UpdateFilterRule validates and persists changes to an existing filter rule.
func (a *App) UpdateFilterRule(rule *mailfilter.Rule) error {
	if rule == nil {
		return fmt.Errorf("filter rule cannot be nil")
	}
	if rule.ID == "" {
		return fmt.Errorf("filter rule ID cannot be empty")
	}
	if err := a.validateFilterRule(rule); err != nil {
		return err
	}
	return a.filterStore.Update(rule)
}

// DeleteFilterRule removes a local cross-account filter rule.
func (a *App) DeleteFilterRule(id string) error {
	if id == "" {
		return fmt.Errorf("filter rule ID cannot be empty")
	}
	return a.filterStore.Delete(id)
}

// ReorderFilterRules saves the priority order of filter rules.
func (a *App) ReorderFilterRules(ids []string) error {
	return a.filterStore.Reorder(ids)
}

// validateFilterRule enforces the cross-account invariant: the destination
// folder must exist and, when the rule is scoped to one source account, it
// must live on a different account. A rule scoped to all accounts may not
// exclude anything; matching messages on the destination account are simply
// never routed (see mailfilter.Match).
func (a *App) validateFilterRule(rule *mailfilter.Rule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("filter rule name cannot be empty")
	}
	if rule.DestFolderID == "" {
		return fmt.Errorf("filter rule destination folder is required")
	}
	dest, err := a.folderStore.Get(rule.DestFolderID)
	if err != nil {
		return fmt.Errorf("failed to load destination folder: %w", err)
	}
	if dest == nil {
		return fmt.Errorf("destination folder not found: %s", rule.DestFolderID)
	}
	if rule.AccountID == "" {
		return nil
	}
	acct, err := a.accountStore.Get(rule.AccountID)
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

// handleNewMessageForFilters applies local cross-account filter rules to a
// newly synced message. Rules are evaluated in priority order and the first
// match wins. Heavy routing work runs in a goroutine so the sync's new-mail
// notification path stays lightweight.
func (a *App) handleNewMessageForFilters(accountID, folderID string, msg *message.Message) {
	log := logging.WithComponent("app.filters")

	rules, err := a.filterStore.ListEnabled()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load enabled filter rules")
		return
	}
	if len(rules) == 0 {
		return
	}

	criteria := mailfilter.FromMessage(msg)
	for _, rule := range rules {
		dest, err := a.folderStore.Get(rule.DestFolderID)
		if err != nil {
			log.Warn().Err(err).Str("ruleID", rule.ID).Msg("Filter rule destination lookup failed")
			continue
		}
		if dest == nil {
			// Destination folder vanished (e.g. deleted on the server); skip
			// the rule until it exists again rather than failing every match.
			log.Debug().Str("ruleID", rule.ID).Str("destFolderID", rule.DestFolderID).Msg("Filter rule destination missing, skipping rule")
			continue
		}
		if !mailfilter.Match(rule, criteria, dest.AccountID) {
			continue
		}
		go a.applyFilterRule(rule, dest, msg)
		return
	}
}

// applyFilterRule routes one message through the standard safe move pipeline:
// the cross-account branch APPENDs the raw message to the destination first
// and only deletes the source once that copy has been confirmed. The move
// runs under the shared per-message mutation lease so a concurrent user move
// of the same message is never raced, and it deliberately records no undo
// entry of its own to keep the undo stack clear of automated moves.
func (a *App) applyFilterRule(rule *mailfilter.Rule, dest *folder.Folder, msg *message.Message) {
	defer recoverPanic("app.filters", "apply filter rule")
	log := logging.WithComponent("app.filters")

	// Re-check current state: the user may have moved or deleted the message
	// between the sync and this goroutine running.
	current, err := a.messageStore.GetByIDs([]string{msg.ID})
	if err != nil || len(current) == 0 {
		log.Debug().Str("messageRef", logging.ShortHash(msg.ID)).Msg("Filter target message no longer exists, skipping")
		return
	}
	if current[0].AccountID != msg.AccountID || current[0].FolderID != msg.FolderID {
		log.Debug().Str("messageRef", logging.ShortHash(msg.ID)).Msg("Filter target message moved since match, skipping")
		return
	}

	_, _, _, err = a.runMoveMutation([]string{msg.ID}, "filter:"+rule.ID, func() (bool, string, error) {
		_, moveErr := a.moveToFolder([]string{msg.ID}, dest.ID, false, "")
		return false, "", moveErr
	})
	if err != nil {
		log.Warn().Err(err).Str("ruleID", rule.ID).Str("destFolder", dest.Path).Msg("Filter rule move failed")
		return
	}
	log.Info().Str("ruleID", rule.ID).Str("destFolder", dest.Path).Msg("Filter rule routed message")
}
