package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/message"
)

// GetPendingMoveFilterSuggestions returns drafts awaiting user review.
func (a *App) GetPendingMoveFilterSuggestions() ([]*mailfilter.SuggestedFilter, error) {
	suggestions, err := a.moveFilterHistory.Pending()
	if err != nil {
		return nil, err
	}
	for _, suggestion := range suggestions {
		acct, accountErr := a.accountStore.Get(suggestion.AccountID)
		dest, destErr := a.folderStore.Get(suggestion.DestFolderID)
		if dest != nil {
			suggestion.DestAccountID = dest.AccountID
		}
		suggestion.RequiresGmailFilter = accountErr == nil && destErr == nil && acct != nil && dest != nil && acct.AuthType == account.AuthOAuth2 && strings.EqualFold(acct.IMAPHost, "imap.gmail.com") && dest.AccountID == acct.ID
	}
	return suggestions, nil
}

// DismissMoveFilterSuggestion suppresses a suggestion without changing mail.
func (a *App) DismissMoveFilterSuggestion(key string) error {
	if key == "" {
		return fmt.Errorf("suggestion key is required")
	}
	return a.moveFilterHistory.SetState(key, "dismissed")
}

// CreateSuggestedMoveFilter creates a local rule only after explicit review.
func (a *App) CreateSuggestedMoveFilter(suggestion *mailfilter.SuggestedFilter, rule *mailfilter.Rule) error {
	if suggestion == nil || rule == nil || suggestion.Key == "" {
		return fmt.Errorf("suggestion and rule are required")
	}
	stored, err := a.moveFilterHistory.GetPending(suggestion.Key)
	if err != nil {
		return fmt.Errorf("suggestion is no longer pending")
	}
	suggestion = stored
	if suggestion.RequiresGmailFilter {
		return fmt.Errorf("same-account Gmail filters are not available yet")
	}
	if suggestion.AccountID != rule.AccountID || suggestion.SourceFolderID == "" || suggestion.DestFolderID != rule.DestFolderID {
		return fmt.Errorf("rule scope does not match suggestion")
	}
	if len(rule.FromContains)+len(rule.ToContains)+len(rule.SubjectContains) == 0 {
		return fmt.Errorf("at least one match criterion is required")
	}
	rule.FolderIDs = []string{suggestion.SourceFolderID}
	if err := a.validateFilterRule(rule); err != nil {
		return err
	}
	if err := a.filterStore.Create(rule); err != nil {
		return err
	}
	return a.moveFilterHistory.SetState(suggestion.Key, "created")
}

// PreviewSuggestedMoveFilter returns matching local message headers only.
func (a *App) PreviewSuggestedMoveFilter(suggestion *mailfilter.SuggestedFilter, rule *mailfilter.Rule) ([]*message.MessageHeader, error) {
	if suggestion == nil || rule == nil {
		return nil, fmt.Errorf("suggestion and rule are required")
	}
	stored, err := a.moveFilterHistory.GetPending(suggestion.Key)
	if err != nil {
		return nil, fmt.Errorf("suggestion is no longer pending")
	}
	suggestion = stored
	destination, err := a.folderStore.Get(suggestion.DestFolderID)
	if err != nil || destination == nil {
		return nil, fmt.Errorf("suggested destination folder is unavailable")
	}
	suggestion.DestAccountID = destination.AccountID
	rows, err := a.db.Query(`SELECT id,account_id,folder_id,uid,subject,from_name,from_email,date,is_read,is_starred,has_attachments,to_list,cc_list FROM messages WHERE account_id=? AND folder_id=? ORDER BY date DESC LIMIT 300`, suggestion.AccountID, suggestion.SourceFolderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*message.MessageHeader
	for rows.Next() {
		var h message.MessageHeader
		var uid int64
		var date, toRaw, ccRaw sql.NullString
		var toList, ccList []message.Address
		if err := rows.Scan(&h.ID, &h.AccountID, &h.FolderID, &uid, &h.Subject, &h.FromName, &h.FromEmail, &date, &h.IsRead, &h.IsStarred, &h.HasAttachments, &toRaw, &ccRaw); err != nil {
			return nil, err
		}
		h.UID = uint32(uid)
		if date.Valid {
			h.Date = parseMessageDate(date.String)
		}
		if toRaw.Valid {
			_ = json.Unmarshal([]byte(toRaw.String), &toList)
		}
		if ccRaw.Valid {
			_ = json.Unmarshal([]byte(ccRaw.String), &ccList)
		}
		criteria := &mailfilter.Message{AccountID: h.AccountID, FolderID: h.FolderID, FromName: h.FromName, FromEmail: h.FromEmail, Subject: h.Subject}
		for _, addr := range append(toList, ccList...) {
			criteria.ToNames = append(criteria.ToNames, addr.Name)
			criteria.ToEmails = append(criteria.ToEmails, addr.Email)
		}
		check := *rule
		check.Enabled = true
		check.AccountID = suggestion.AccountID
		check.FolderIDs = []string{suggestion.SourceFolderID}
		check.DestFolderID = suggestion.DestFolderID
		if mailfilter.Match(&check, criteria, destination.AccountID, suggestion.DestFolderID) {
			out = append(out, &h)
			if len(out) >= 20 {
				break
			}
		}
	}
	return out, rows.Err()
}

func parseMessageDate(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if value, err := time.Parse(layout, raw); err == nil {
			return value
		}
	}
	return time.Time{}
}

// ============================================================================
// Local Cross-Account Filter API - Exposed to frontend via Wails bindings
// ============================================================================

// GetFilterRules returns all local filter rules in priority order.
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

// validateFilterRule checks destination existence and the explicit source
// scope required for safe same-account non-Gmail routing.
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
		if len(rule.FolderIDs) == 0 {
			return fmt.Errorf("same-account rules require an explicit source folder")
		}
		for _, sourceFolderID := range rule.FolderIDs {
			if sourceFolderID == rule.DestFolderID {
				return fmt.Errorf("source scope cannot include the destination folder")
			}
		}
		if acct.AuthType == account.AuthOAuth2 && strings.EqualFold(acct.IMAPHost, "imap.gmail.com") {
			return fmt.Errorf("same-account Gmail rules require Gmail filter support")
		}
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
		if !mailfilter.Match(rule, criteria, dest.AccountID, dest.ID) {
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
