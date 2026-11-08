package alwayson

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

type catalogReadOperations struct {
	accounts       *account.Store
	folders        *folder.Store
	virtualFolders *virtualfolder.Store
	messages       *message.Store
}

func (o *catalogReadOperations) GetAccounts() ([]*account.Account, error) {
	return o.accounts.List()
}

func (o *catalogReadOperations) GetFolders(accountID string) ([]*folder.Folder, error) {
	return o.folders.List(accountID)
}

func (o *catalogReadOperations) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) {
	return o.virtualFolders.List()
}

func (o *catalogReadOperations) GetFolder(folderID string) (*folder.Folder, error) {
	return o.folders.Get(folderID)
}

func (o *catalogReadOperations) ListByFolder(folderID string, offset, limit int) ([]*message.MessageHeader, error) {
	return o.messages.ListByFolder(folderID, offset, limit)
}

func (o *catalogReadOperations) ListConversationsByFolder(folderID string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	return o.messages.ListConversationsByFolder(folderID, offset, limit, sortOrder, filter)
}

func (o *catalogReadOperations) ListConversationsUnifiedInbox(offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, folder.TypeInbox)
	if err != nil {
		return nil, err
	}
	return o.messages.ListConversationsByFolderIDs(folderIDs, offset, limit, sortOrder, filter)
}

func (r *SyncRuntime) CreateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := r.virtualFolders.Create(vf); err != nil {
		return err
	}
	return r.refreshVirtualFolderCache(vf.ID)
}

func (r *SyncRuntime) UpdateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := r.virtualFolders.Update(vf); err != nil {
		return err
	}
	return r.refreshVirtualFolderCache(vf.ID)
}

func (r *SyncRuntime) DeleteVirtualFolder(id string) error {
	if r == nil || r.virtualFolders == nil {
		return errors.New("always-on runtime is not open")
	}
	return r.virtualFolders.Delete(id)
}

func (r *SyncRuntime) ReorderVirtualFolders(ids []string) error {
	if r == nil || r.virtualFolders == nil {
		return errors.New("always-on runtime is not open")
	}
	return r.virtualFolders.Reorder(ids)
}

func (r *SyncRuntime) RefreshVirtualFolderCache(id string) error {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return errors.New("always-on runtime is not open")
	}
	return r.refreshVirtualFolderCache(id)
}

func (r *SyncRuntime) refreshVirtualFolderCache(id string) error {
	vf, err := r.virtualFolders.Get(id)
	if err != nil {
		return fmt.Errorf("load virtual folder for cache refresh: %w", err)
	}
	if vf == nil {
		return fmt.Errorf("virtual folder not found: %s", id)
	}
	folderIDs, err := r.virtualFolders.QueryFolderIDs(vf)
	if err != nil {
		return fmt.Errorf("resolve virtual folder members: %w", err)
	}
	refs, err := r.messages.ListVirtualFolderConversationIDs(folderIDs, vf.SearchQuery, 200, message.VirtualFolderCriteria{
		ReadStatus: vf.ReadStatus, ReplyStatus: vf.ReplyStatus, StarStatus: vf.StarStatus, DateWindow: vf.DateWindow,
	})
	if err != nil {
		return fmt.Errorf("query virtual folder cache members: %w", err)
	}
	entries := make([]virtualfolder.CachedConversation, 0, len(refs))
	for _, ref := range refs {
		entries = append(entries, virtualfolder.CachedConversation{AccountID: ref.AccountID, ConversationID: ref.ConversationID})
	}
	return r.virtualFolders.SaveCache(id, entries)
}

func (r *SyncRuntime) GetFilterRules() ([]*mailfilter.Rule, error) {
	if r == nil || r.rules == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	return r.rules.List()
}

func (r *SyncRuntime) CreateFilterRule(rule *mailfilter.Rule) error {
	if r == nil || r.rules == nil || r.accounts == nil || r.folders == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := mailfilter.ValidateRule(rule, r.accounts, r.folders); err != nil {
		return err
	}
	if rule.OrderIndex == 0 {
		if existing, err := r.rules.List(); err == nil {
			rule.OrderIndex = len(existing)
		}
	}
	return r.rules.Create(rule)
}

func (r *SyncRuntime) UpdateFilterRule(rule *mailfilter.Rule) error {
	if r == nil || r.rules == nil || r.accounts == nil || r.folders == nil {
		return errors.New("always-on runtime is not open")
	}
	if rule == nil || rule.ID == "" {
		return errors.New("filter rule ID cannot be empty")
	}
	if err := mailfilter.ValidateRule(rule, r.accounts, r.folders); err != nil {
		return err
	}
	return r.rules.Update(rule)
}

func (r *SyncRuntime) DeleteFilterRule(id string) error {
	if r == nil || r.rules == nil {
		return errors.New("always-on runtime is not open")
	}
	return r.rules.Delete(id)
}

func (r *SyncRuntime) ReorderFilterRules(ids []string) error {
	if r == nil || r.rules == nil {
		return errors.New("always-on runtime is not open")
	}
	return r.rules.Reorder(ids)
}

// SyncFolders refreshes one account's provider folder catalog in the helper.
func (r *SyncRuntime) SyncFolders(ctx context.Context, accountID string) error {
	if r == nil || r.syncEngine == nil || r.accounts == nil {
		return errors.New("always-on runtime is not open")
	}
	if strings.TrimSpace(accountID) == "" {
		return errors.New("account ID is required")
	}
	account, err := r.accounts.Get(accountID)
	if err != nil {
		return fmt.Errorf("load account for folder sync: %w", err)
	}
	if account == nil {
		return fmt.Errorf("account not found: %s", accountID)
	}
	return r.syncEngine.SyncFolders(ctx, accountID)
}

// SetFolderSubscription applies provider subscription state and updates the helper cache.
func (r *SyncRuntime) SetFolderSubscription(ctx context.Context, accountID, folderID string, subscribed bool) error {
	if r == nil || r.syncEngine == nil || r.folders == nil || r.accounts == nil {
		return errors.New("always-on runtime is not open")
	}
	account, err := r.accounts.Get(accountID)
	if err != nil {
		return fmt.Errorf("load account for folder subscription: %w", err)
	}
	if account == nil {
		return fmt.Errorf("account not found: %s", accountID)
	}
	f, err := r.folders.Get(folderID)
	if err != nil {
		return fmt.Errorf("load folder for subscription: %w", err)
	}
	if f == nil || f.AccountID != accountID {
		return errors.New("folder not found for account")
	}

	client, apiErr := r.syncEngine.GmailClient(ctx, accountID)
	if apiErr == nil {
		label, err := r.syncEngine.GmailLabelID(ctx, client, f)
		if err != nil {
			return err
		}
		if label != "" {
			if err := client.SetLabelVisible(ctx, label, subscribed); err != nil {
				return fmt.Errorf("update Gmail folder visibility: %w", err)
			}
		}
		return r.folders.UpdateSubscribed(folderID, subscribed)
	}
	if !errors.Is(apiErr, gmail.ErrNotCapable) {
		return fmt.Errorf("prepare Gmail folder subscription: %w", apiErr)
	}

	connection, err := r.syncEngine.GetPoolConnection(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get IMAP connection for folder subscription: %w", err)
	}
	defer r.syncEngine.ReleasePoolConnection(connection)
	if subscribed {
		err = connection.Client().Subscribe(f.Path)
	} else {
		err = connection.Client().Unsubscribe(f.Path)
	}
	if err != nil {
		return fmt.Errorf("update IMAP folder subscription: %w", err)
	}
	return r.folders.UpdateSubscribed(folderID, subscribed)
}

// SetAllFolderSubscriptions applies the provider-specific bulk subscription behavior.
func (r *SyncRuntime) SetAllFolderSubscriptions(ctx context.Context, accountID string, subscribed bool) error {
	if r == nil || r.syncEngine == nil || r.folders == nil || r.accounts == nil {
		return errors.New("always-on runtime is not open")
	}
	account, err := r.accounts.Get(accountID)
	if err != nil {
		return fmt.Errorf("load account for folder subscriptions: %w", err)
	}
	if account == nil {
		return fmt.Errorf("account not found: %s", accountID)
	}
	folders, err := r.folders.List(accountID)
	if err != nil {
		return fmt.Errorf("list folders for account: %w", err)
	}
	log := logging.WithComponent("alwayson.folders")
	client, apiErr := r.syncEngine.GmailClient(ctx, accountID)
	if apiErr == nil {
		for _, f := range folders {
			label, err := r.syncEngine.GmailLabelID(ctx, client, f)
			if err != nil {
				return err
			}
			if label != "" {
				if err := client.SetLabelVisible(ctx, label, subscribed); err != nil {
					return fmt.Errorf("update Gmail folder visibility: %w", err)
				}
			}
			if err := r.folders.UpdateSubscribed(f.ID, subscribed); err != nil {
				return err
			}
		}
		return nil
	}
	if !errors.Is(apiErr, gmail.ErrNotCapable) {
		return fmt.Errorf("prepare Gmail folder subscriptions: %w", apiErr)
	}
	connection, err := r.syncEngine.GetPoolConnection(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get IMAP connection for folder subscriptions: %w", err)
	}
	defer r.syncEngine.ReleasePoolConnection(connection)
	for _, f := range folders {
		if subscribed {
			err = connection.Client().Subscribe(f.Path)
		} else {
			err = connection.Client().Unsubscribe(f.Path)
		}
		if err != nil {
			log.Warn().Err(err).Str("folder", f.Path).Msg("Failed to update IMAP folder subscription")
			continue
		}
		if err := r.folders.UpdateSubscribed(f.ID, subscribed); err != nil {
			log.Warn().Err(err).Str("folder", f.Path).Msg("Failed to update folder subscription cache")
		}
	}
	return nil
}

// ListUncertainFilterJobs returns durable automatic moves for inspection only.
func (r *SyncRuntime) ListUncertainFilterJobs(offset, limit int) ([]mailfilter.FilterJobInspection, error) {
	if r == nil || r.filterJobs == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	return r.filterJobs.ListOutcomeUnknown(offset, limit)
}

// CatalogAPIHandler is kept as the compatible constructor for the mailbox read
// API handler. Mount the result under the authenticated server's /v1/ prefix.
func (r *SyncRuntime) CatalogAPIHandler() (http.Handler, error) {
	return r.MailboxReadAPIHandler()
}

// MailboxReadAPIHandler creates the headless account, folder, saved-view, and
// bounded message-header routes. Mount it under the authenticated automation
// server's /v1/ prefix; this handler has no listener or authentication layer.
func (r *SyncRuntime) MailboxReadAPIHandler() (http.Handler, error) {
	if r == nil || r.db == nil || r.mailService == nil || r.syncEngine == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	operations := &catalogReadOperations{
		accounts:       r.accounts,
		folders:        r.folders,
		virtualFolders: r.virtualFolders,
		messages:       r.messages,
	}
	readOperations := &mailboxReadOperations{runtime: r}
	mux := http.NewServeMux()
	v1.MountCatalogRoutes(mux, operations, automation.RequireGrantCapability)
	v1.NewAccountMutationHandler(r, newAccountMutationLedger(r.db), r.accountMutationFingerprint).MountRoutes(mux, automation.RequireGrantCapability)
	v1.MountFolderMutationRoutes(mux, r, automation.RequireGrantCapability)
	v1.MountVirtualFolderMutationRoutes(mux, r, automation.RequireGrantCapability)
	v1.MountFilterRuleRoutes(mux, r, automation.RequireGrantCapability)
	v1.MountFilterJobRoutes(mux, r, automation.RequireGrantCapability)
	v1.MountReplyLaterRoutes(mux, r, automation.RequireGrantCapability)
	v1.MountTriageRoutes(mux, r, automation.RequireGrantCapability)
	v1.MountMessageHeaderRoutes(mux, operations, automation.RequireGrantCapability)
	v1.MountConversationListRoute(mux, operations, automation.RequireGrantCapability)
	v1.MountSearchRoute(mux, readOperations, automation.RequireGrantCapability)
	v1.MountThreadBodyRoutes(mux, readOperations, automation.RequireGrantCapability)
	v1.NewActionsHandler(&mailboxActionOperations{runtime: r}).MountRoutes(mux, automation.RequireGrantCapability)
	return mux, nil
}
