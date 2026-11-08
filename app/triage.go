package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/triage"
)

const triageLabelPrefix = "Postilaatikko/Triage/"

func (a *App) setGmailTriageLabel(accountID, conversationID, queue string, enabled bool) error {
	acc, err := a.accountStore.Get(accountID)
	if err != nil {
		return err
	}
	if acc == nil || acc.AuthType != account.AuthOAuth2 || acc.IMAPHost != gmailIMAPHost {
		return nil
	}
	if _, err := a.gmailTokensFor(accountID, gmail.ScopesCoverModify, false); err != nil {
		if err == gmail.ErrNotCapable {
			return nil
		}
		return fmt.Errorf("Gmail triage labels require Gmail modify access: %w", err)
	}
	if strings.TrimSpace(conversationID) == "" {
		return fmt.Errorf("conversation ID is required")
	}
	a.triageLabelMu.Lock()
	defer a.triageLabelMu.Unlock()
	threadID := conversationID
	gmailPrefix := "gmail:" + accountID + ":"
	threadID = strings.TrimPrefix(threadID, gmailPrefix)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := a.newGmailModifyClient(accountID)
	var labelID string
	lookupErr := a.db.QueryRowContext(ctx, `SELECT label_id FROM gmail_triage_labels WHERE account_id=? AND queue=?`, accountID, queue).Scan(&labelID)
	if lookupErr != nil && lookupErr != sql.ErrNoRows {
		return fmt.Errorf("load Gmail triage label: %w", lookupErr)
	}
	if labelID == "" {
		name := triageLabelPrefix + map[string]string{"read-later": "Read Later", "reply-later": "Reply Later"}[queue]
		if name == triageLabelPrefix {
			return fmt.Errorf("unsupported Gmail triage queue: %s", queue)
		}
		labels, err := c.Labels(ctx)
		if err != nil {
			return fmt.Errorf("list Gmail labels: %w", err)
		}
		for _, item := range labels {
			if item.Name == name && item.Type == "user" {
				labelID = item.ID
				break
			}
		}
		if labelID == "" {
			created, err := c.CreateHiddenUserLabel(ctx, name)
			if err != nil {
				return fmt.Errorf("create hidden Gmail triage label: %w", err)
			}
			labelID = created.ID
		}
		if _, err := a.db.ExecContext(ctx, `INSERT INTO gmail_triage_labels(account_id,queue,label_id) VALUES(?,?,?) ON CONFLICT(account_id,queue) DO UPDATE SET label_id=excluded.label_id`, accountID, queue, labelID); err != nil {
			return fmt.Errorf("save Gmail triage label: %w", err)
		}
	}
	var add, remove []string
	if enabled {
		add = []string{labelID}
	} else {
		remove = []string{labelID}
	}
	if err := c.ModifyThread(ctx, threadID, add, remove); err != nil {
		return fmt.Errorf("update Gmail triage label: %w", err)
	}
	return nil
}

// syncGmailTriageQueue refreshes one account's local index from the hidden
// Gmail label. The user label is authoritative for Gmail; non-Gmail accounts
// never enter this path and keep their app-local queue membership.
func (a *App) syncGmailTriageQueue(accountID, queue string) error {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := a.gmailTokensFor(accountID, gmail.ScopesCoverModify, false); err != nil {
		return err
	}
	var labelID string
	if err := a.db.QueryRowContext(ctx, `SELECT label_id FROM gmail_triage_labels WHERE account_id=? AND queue=?`, accountID, queue).Scan(&labelID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	c := a.newGmailModifyClient(accountID)
	var pageToken string
	threads := map[string]struct{}{}
	for {
		page, err := c.Search(ctx, gmail.SearchRequest{LabelIDs: []string{labelID}, PageToken: pageToken, Limit: 500, IncludeSpamTrash: true})
		if err != nil {
			return err
		}
		for start := 0; start < len(page.MessageIDs); start += 100 {
			end := start + 100
			if end > len(page.MessageIDs) {
				end = len(page.MessageIDs)
			}
			items, err := c.Metadata(ctx, page.MessageIDs[start:end])
			if err != nil {
				return err
			}
			for _, item := range items {
				id := item.ThreadID
				if id == "" {
					id = item.ID
				}
				if id != "" {
					threads["gmail:"+accountID+":"+id] = struct{}{}
				}
			}
		}
		pageToken = page.NextPageToken
		if pageToken == "" {
			break
		}
	}
	table := "read_later_queue"
	if queue == "reply-later" {
		table = "reply_later_queue"
	} else if queue != "read-later" {
		return fmt.Errorf("unsupported Gmail triage queue: %s", queue)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE account_id=?`, accountID); err != nil {
		return err
	}
	for threadID := range threads {
		if _, err = tx.ExecContext(ctx, `INSERT INTO `+table+`(account_id,conversation_id) VALUES(?,?) ON CONFLICT DO NOTHING`, accountID, threadID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	event := "readLater:synced"
	if queue == "reply-later" {
		event = "replyLater:synced"
	}
	a.emitRuntimeEvent(event, map[string]interface{}{"accountId": accountID})
	return nil
}

func (a *App) syncGmailTriageQueues(queue string) {
	accounts, err := a.accountStore.List()
	if err != nil {
		return
	}
	for _, acc := range accounts {
		if !acc.Enabled || acc.AuthType != account.AuthOAuth2 || acc.IMAPHost != gmailIMAPHost {
			continue
		}
		if err := a.syncGmailTriageQueue(acc.ID, queue); err != nil {
			continue
		}
	}
}

// GetReadLaterQueue exposes explicit triage membership from the mailbox service when available.
func (a *App) GetReadLaterQueue() ([]triage.Ref, error) {
	if a.mailboxService != nil {
		ctx := a.helperCommandContext()
		status := a.mailboxService.status(ctx)
		if status.Connected {
			return a.mailboxService.readLaterQueue(ctx)
		}
		if status.ErrorCode != "service_unavailable" {
			return nil, fmt.Errorf("load Read Later queue from mailbox service: %s", status.Message)
		}
	}
	if a.triageStore == nil {
		return nil, fmt.Errorf("Read Later queue is not ready")
	}
	return a.triageStore.ListReadLater()
}

func (a *App) GetReadLaterCount() (int, error) {
	queue, err := a.GetReadLaterQueue()
	return len(queue), err
}
func (a *App) IsReadLater(accountID, conversationID string) (bool, error) {
	queue, err := a.GetReadLaterQueue()
	if err != nil {
		return false, err
	}
	for _, ref := range queue {
		if ref.AccountID == accountID && ref.ConversationID == conversationID {
			return true, nil
		}
	}
	return false, nil
}

func (a *App) ChangeReadLater(accountID, conversationID string, inQueue bool) (bool, error) {
	ref := triage.Ref{AccountID: accountID, ConversationID: conversationID}
	changed, handled, err := a.changeReadLaterThroughService(ref, inQueue)
	if handled {
		if err != nil {
			return false, err
		}
	} else {
		if err := a.setGmailTriageLabel(accountID, conversationID, "read-later", inQueue); err != nil {
			return false, err
		}
		if a.triageStore == nil {
			return false, fmt.Errorf("Read Later queue is not ready")
		}
		changed, err = a.triageStore.SetReadLater(ref, inQueue)
	}
	if err == nil && changed {
		a.emitRuntimeEvent("readLater:changed", map[string]interface{}{"accountId": accountID, "conversationId": conversationID, "inQueue": inQueue})
	}
	return changed, err
}

func (a *App) changeReadLaterThroughService(ref triage.Ref, inQueue bool) (bool, bool, error) {
	if a.mailboxService == nil {
		return false, false, nil
	}
	ctx := a.helperCommandContext()
	status := a.mailboxService.status(ctx)
	if !status.Connected {
		if status.ErrorCode == "service_unavailable" {
			return false, false, nil
		}
		return false, true, fmt.Errorf("mailbox service is not ready for Read Later: %s", status.Message)
	}
	if !status.WriteAuthorized {
		return false, true, errDesktopMailboxWriteAuthorizationRequired
	}
	changed, err := a.mailboxService.setReadLater(ctx, ref, inQueue)
	if err != nil {
		return false, true, fmt.Errorf("update Read Later through local mailbox service: %w", err)
	}
	return changed, true, nil
}

func (a *App) AddReadLater(accountID, conversationID string) (bool, error) {
	return a.ChangeReadLater(accountID, conversationID, true)
}
func (a *App) RemoveReadLater(accountID, conversationID string) (bool, error) {
	return a.ChangeReadLater(accountID, conversationID, false)
}

// SnoozeConversation stores a local wake deadline without changing provider
// read state, labels, or folders.
func (a *App) SnoozeConversation(accountID, conversationID, wakeAt string) error {
	deadline, err := time.Parse(time.RFC3339Nano, wakeAt)
	if err != nil {
		return fmt.Errorf("invalid snooze deadline: %w", err)
	}
	ref := triage.Ref{AccountID: accountID, ConversationID: conversationID}
	handled := false
	if a.mailboxService != nil {
		ctx := a.helperCommandContext()
		status := a.mailboxService.status(ctx)
		if status.Connected {
			if !status.WriteAuthorized {
				return errDesktopMailboxWriteAuthorizationRequired
			}
			if err := a.mailboxService.snooze(ctx, ref, deadline.UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("snooze through local mailbox service: %w", err)
			}
			handled = true
		} else if status.ErrorCode != "service_unavailable" {
			return fmt.Errorf("mailbox service is not ready for Snooze: %s", status.Message)
		}
	}
	if !handled {
		if a.triageStore == nil {
			return fmt.Errorf("Snooze store is not ready")
		}
		if err := a.triageStore.Snooze(ref, deadline); err != nil {
			return err
		}
	}
	a.emitRuntimeEvent("snooze:changed", map[string]interface{}{"accountId": accountID, "conversationId": conversationID, "wakeAt": deadline.UTC().Format(time.RFC3339Nano)})
	return nil
}
func (a *App) UnsnoozeConversation(accountID, conversationID string) (bool, error) {
	ref := triage.Ref{AccountID: accountID, ConversationID: conversationID}
	changed, handled, err := a.unsnoozeThroughService(ref)
	if handled {
		if err != nil {
			return false, err
		}
	} else {
		if a.triageStore == nil {
			return false, fmt.Errorf("Snooze store is not ready")
		}
		changed, err = a.triageStore.Unsnooze(accountID, conversationID)
	}
	if err == nil && changed {
		a.emitRuntimeEvent("snooze:changed", map[string]interface{}{"accountId": accountID, "conversationId": conversationID, "wakeAt": ""})
	}
	return changed, err
}

func (a *App) unsnoozeThroughService(ref triage.Ref) (bool, bool, error) {
	if a.mailboxService == nil {
		return false, false, nil
	}
	ctx := a.helperCommandContext()
	status := a.mailboxService.status(ctx)
	if !status.Connected {
		if status.ErrorCode == "service_unavailable" {
			return false, false, nil
		}
		return false, true, fmt.Errorf("mailbox service is not ready for Snooze: %s", status.Message)
	}
	if !status.WriteAuthorized {
		return false, true, errDesktopMailboxWriteAuthorizationRequired
	}
	changed, err := a.mailboxService.unsnooze(ctx, ref)
	if err != nil {
		return false, true, fmt.Errorf("unsnooze through local mailbox service: %w", err)
	}
	return changed, true, nil
}

func (a *App) GetSnoozedConversations() ([]triage.Ref, error) {
	if a.mailboxService != nil {
		ctx := a.helperCommandContext()
		status := a.mailboxService.status(ctx)
		if status.Connected {
			return a.mailboxService.snoozedConversations(ctx)
		}
		if status.ErrorCode != "service_unavailable" {
			return nil, fmt.Errorf("load Snoozed conversations from mailbox service: %s", status.Message)
		}
	}
	if a.triageStore == nil {
		return nil, fmt.Errorf("Snooze store is not ready")
	}
	return a.triageStore.ListSnoozed()
}

func (a *App) GetSnoozedCount() (int, error) {
	if a.mailboxService != nil {
		ctx := a.helperCommandContext()
		status := a.mailboxService.status(ctx)
		if status.Connected {
			return a.mailboxService.snoozedCount(ctx)
		}
		if status.ErrorCode != "service_unavailable" {
			return 0, fmt.Errorf("count Snoozed conversations from mailbox service: %s", status.Message)
		}
	}
	if a.triageStore == nil {
		return 0, fmt.Errorf("Snooze store is not ready")
	}
	return a.triageStore.CountSnoozed()
}

func (a *App) runSnoozeWakeLoop() {
	ctx := a.ctx
	if ctx == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	type snoozeKey struct{ accountID, conversationID string }
	known := map[snoozeKey]time.Time{}
	if current, err := a.GetSnoozedConversations(); err == nil {
		for _, ref := range current {
			known[snoozeKey{ref.AccountID, ref.ConversationID}] = ref.WakeAt
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			status := MailboxServiceStatus{ErrorCode: "service_unavailable"}
			if a.mailboxService != nil {
				status = a.mailboxService.status(ctx)
			}
			if !status.Connected && status.ErrorCode == "service_unavailable" {
				count, err := a.triageStore.ExpireSnoozes(now)
				if err == nil && count > 0 {
					a.emitRuntimeEvent("snooze:expired", map[string]interface{}{"count": count})
				}
				current, err := a.triageStore.ListSnoozed()
				if err == nil {
					known = make(map[snoozeKey]time.Time, len(current))
					for _, ref := range current {
						known[snoozeKey{ref.AccountID, ref.ConversationID}] = ref.WakeAt
					}
				}
				continue
			}
			if !status.Connected {
				continue
			}
			current, err := a.mailboxService.snoozedConversations(ctx)
			if err != nil {
				continue
			}
			currentSet := make(map[snoozeKey]time.Time, len(current))
			for _, ref := range current {
				currentSet[snoozeKey{ref.AccountID, ref.ConversationID}] = ref.WakeAt
			}
			var expired int
			for key, wakeAt := range known {
				if !wakeAt.After(now) {
					if _, exists := currentSet[key]; !exists {
						expired++
					}
				}
			}
			if expired > 0 {
				a.emitRuntimeEvent("snooze:expired", map[string]interface{}{"count": expired})
			}
			known = currentSet
		}
	}
}

func (a *App) changeReplyLaterProvider(accountID, conversationID string, inQueue bool) error {
	return a.setGmailTriageLabel(accountID, conversationID, "reply-later", inQueue)
}
