package alwayson

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/automation"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/logging"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/operations"
	syncengine "github.com/ollisulopuisto/postilaatikko/internal/sync"
	"github.com/ollisulopuisto/postilaatikko/internal/triage"
	"github.com/ollisulopuisto/postilaatikko/internal/virtualfolder"
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

func (o *catalogReadOperations) GetIdentities(accountID string) ([]*account.Identity, error) {
	if _, err := o.accounts.Get(accountID); err != nil {
		return nil, err
	}
	return o.accounts.GetIdentities(accountID)
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

// Keyset conversation pagination (PERF-SC-005). The store seeks the page
// cursor through idx_messages_folder_date_conv, so unfiltered newest-first
// pages no longer group and sort the full folder per request.

// ListConversationsByFolderKeyset returns a keyset page for an eligible physical folder, retaining legacy fallbacks.
func (o *catalogReadOperations) ListConversationsByFolderKeyset(folderID, cursor string, limit int) ([]*message.Conversation, string, error) {
	return o.messages.ListConversationsByFolderKeyset(folderID, cursor, limit)
}

// ListConversationsUnifiedInboxKeyset returns a keyset page across enabled account inboxes.
func (o *catalogReadOperations) ListConversationsUnifiedInboxKeyset(cursor string, limit int) ([]*message.Conversation, string, error) {
	folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, folder.TypeInbox)
	if err != nil {
		return nil, "", err
	}
	return o.messages.ListConversationsByFolderIDsKeyset(folderIDs, cursor, limit, "newest", false)
}

// ListConversationsUnifiedFolderKeyset returns a keyset page across a supported unified folder type.
func (o *catalogReadOperations) ListConversationsUnifiedFolderKeyset(folderType, cursor string, limit int) ([]*message.Conversation, string, error) {
	typeValue, err := parseUnifiedFolderType(folderType)
	if err != nil {
		return nil, "", err
	}
	folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, typeValue)
	if err != nil {
		return nil, "", err
	}
	return o.messages.ListConversationsByFolderIDsKeyset(folderIDs, cursor, limit, "newest", false)
}

func (o *catalogReadOperations) CountConversationsByFolder(accountID, folderID, filter string) (int, error) {
	f, err := o.folders.Get(folderID)
	if err != nil {
		return 0, err
	}
	if f == nil || f.AccountID != accountID {
		return 0, errors.New("folder not found for account")
	}
	return o.messages.CountConversationsByFolder(folderID, filter)
}

func (o *catalogReadOperations) ListConversationsUnifiedInbox(offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, folder.TypeInbox)
	if err != nil {
		return nil, err
	}
	return o.messages.ListConversationsByFolderIDs(folderIDs, offset, limit, sortOrder, filter)
}

func (o *catalogReadOperations) ListConversationsUnifiedFolder(folderType string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error) {
	if folderType == "triage" {
		items, err := triage.DefaultFolderScope(o.accounts, o.folders)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		return o.messages.ListConversationsByFolderIDs(ids, offset, limit, sortOrder, filter)
	}
	if folderType == "readLater" || folderType == "replyLater" {
		return o.messages.ListReadLaterConversations(offset, limit, sortOrder, filter)
	}
	typeValue, err := parseUnifiedFolderType(folderType)
	if err != nil {
		return nil, err
	}
	if typeValue != folder.TypeArchive {
		folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, typeValue)
		if err != nil {
			return nil, err
		}
		return o.messages.ListConversationsByFolderIDs(folderIDs, offset, limit, sortOrder, filter)
	}
	physicalIDs, allMailIDs, err := o.unifiedArchiveFolderIDs()
	if err != nil {
		return nil, err
	}
	window := offset + limit
	physical, err := o.messages.ListConversationsByFolderIDs(physicalIDs, 0, window, sortOrder, filter)
	if err != nil {
		return nil, err
	}
	virtual, err := o.messages.ListVirtualArchivedConversations(allMailIDs, 0, window, sortOrder, filter)
	if err != nil {
		return nil, err
	}
	combined := append(physical, virtual...)
	sort.SliceStable(combined, func(i, j int) bool {
		if combined[i].LatestDate.Equal(combined[j].LatestDate) {
			if combined[i].AccountID == combined[j].AccountID {
				return combined[i].ThreadID < combined[j].ThreadID
			}
			return combined[i].AccountID < combined[j].AccountID
		}
		if sortOrder == "oldest" {
			return combined[i].LatestDate.Before(combined[j].LatestDate)
		}
		return combined[i].LatestDate.After(combined[j].LatestDate)
	})
	if offset >= len(combined) {
		return []*message.Conversation{}, nil
	}
	end := offset + limit
	if end > len(combined) {
		end = len(combined)
	}
	return combined[offset:end], nil
}

func (o *catalogReadOperations) CountConversationsUnifiedFolder(folderType, filter string) (int, error) {
	if folderType == "triage" {
		items, err := triage.DefaultFolderScope(o.accounts, o.folders)
		if err != nil {
			return 0, err
		}
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		return o.messages.CountConversationsByFolderIDs(ids, filter)
	}
	if folderType == "readLater" || folderType == "replyLater" {
		return o.messages.CountReadLaterConversations(filter)
	}
	typeValue, err := parseUnifiedFolderType(folderType)
	if err != nil {
		return 0, err
	}
	if typeValue != folder.TypeArchive {
		folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, typeValue)
		if err != nil {
			return 0, err
		}
		return o.messages.CountConversationsByFolderIDs(folderIDs, filter)
	}
	physicalIDs, allMailIDs, err := o.unifiedArchiveFolderIDs()
	if err != nil {
		return 0, err
	}
	physical, err := o.messages.CountConversationsByFolderIDs(physicalIDs, filter)
	if err != nil {
		return 0, err
	}
	virtual, err := o.messages.CountVirtualArchivedConversations(allMailIDs, filter)
	return physical + virtual, err
}

func (o *catalogReadOperations) SearchConversationsUnifiedFolder(folderType, query string, offset, limit int, filter string) ([]*message.ConversationSearchResult, int, error) {
	if folderType == "triage" {
		items, err := triage.DefaultFolderScope(o.accounts, o.folders)
		if err != nil {
			return nil, 0, err
		}
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		return o.messages.SearchConversationsByFolderIDs(ids, query, offset, limit, filter)
	}
	if folderType == "readLater" || folderType == "replyLater" {
		folderIDs := []string{}
		accounts, err := o.accounts.List()
		if err != nil {
			return nil, 0, err
		}
		for _, account := range accounts {
			if !account.Enabled {
				continue
			}
			folders, err := o.folders.List(account.ID)
			if err != nil {
				return nil, 0, err
			}
			for _, folder := range folders {
				folderIDs = append(folderIDs, folder.ID)
			}
		}
		if filter == "" {
			filter = "readLater"
		} else if filter != "readLater" {
			filter = "readLater+" + filter
		}
		return o.messages.SearchConversationsByFolderIDs(folderIDs, query, offset, limit, filter)
	}
	typeValue, err := parseUnifiedFolderType(folderType)
	if err != nil {
		return nil, 0, err
	}
	if typeValue != folder.TypeArchive {
		folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, typeValue)
		if err != nil {
			return nil, 0, err
		}
		return o.messages.SearchConversationsByFolderIDs(folderIDs, query, offset, limit, filter)
	}
	physicalIDs, allMailIDs, err := o.unifiedArchiveFolderIDs()
	if err != nil {
		return nil, 0, err
	}
	window := offset + limit
	physical, physicalCount, err := o.messages.SearchConversationsByFolderIDs(physicalIDs, query, 0, window, filter)
	if err != nil {
		return nil, 0, err
	}
	virtual, virtualCount, err := o.messages.SearchVirtualArchivedConversations(allMailIDs, query, 0, window, filter)
	if err != nil {
		return nil, 0, err
	}
	combined := append(physical, virtual...)
	sort.SliceStable(combined, func(i, j int) bool {
		if combined[i].LatestDate.Equal(combined[j].LatestDate) {
			if combined[i].AccountID == combined[j].AccountID {
				return combined[i].ThreadID < combined[j].ThreadID
			}
			return combined[i].AccountID < combined[j].AccountID
		}
		return combined[i].LatestDate.After(combined[j].LatestDate)
	})
	if offset >= len(combined) {
		return []*message.ConversationSearchResult{}, physicalCount + virtualCount, nil
	}
	end := offset + limit
	if end > len(combined) {
		end = len(combined)
	}
	return combined[offset:end], physicalCount + virtualCount, nil
}

func (o *catalogReadOperations) unifiedArchiveFolderIDs() ([]string, []string, error) {
	accounts, err := o.accounts.List()
	if err != nil {
		return nil, nil, err
	}
	physicalIDs := make([]string, 0, len(accounts))
	allMailIDs := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || !account.Enabled {
			continue
		}
		physical, err := resolveSpecialFolder(o.accounts, o.folders, account.ID, folder.TypeArchive)
		if err != nil {
			return nil, nil, err
		}
		if physical != nil {
			physicalIDs = append(physicalIDs, physical.ID)
			continue
		}
		allMail, err := resolveSpecialFolder(o.accounts, o.folders, account.ID, folder.TypeAll)
		if err != nil {
			return nil, nil, err
		}
		if allMail != nil {
			allMailIDs = append(allMailIDs, allMail.ID)
		}
	}
	return physicalIDs, allMailIDs, nil
}

func parseUnifiedFolderType(value string) (folder.Type, error) {
	typeValue := folder.Type(value)
	switch typeValue {
	case folder.TypeDrafts, folder.TypeSent, folder.TypeTrash, folder.TypeStarred, folder.TypeArchive, folder.TypeSpam:
		return typeValue, nil
	default:
		return "", fmt.Errorf("unsupported unified folder type: %s", value)
	}
}

func (o *catalogReadOperations) CountConversationsUnifiedInbox(filter string) (int, error) {
	folderIDs, err := resolveSpecialFolderIDs(o.accounts, o.folders, folder.TypeInbox)
	if err != nil {
		return 0, err
	}
	return o.messages.CountConversationsByFolderIDs(folderIDs, filter)
}

func (o *catalogReadOperations) CountConversationsVirtualFolder(virtualFolderID string) (int, error) {
	vf, err := o.virtualFolders.Get(virtualFolderID)
	if err != nil {
		return 0, err
	}
	if vf == nil {
		return 0, errors.New("virtual folder not found")
	}
	folderIDs, err := o.virtualFolders.QueryFolderIDs(vf)
	if err != nil {
		return 0, err
	}
	criteria := message.VirtualFolderCriteria{ReadStatus: vf.ReadStatus, ReplyStatus: vf.ReplyStatus, StarStatus: vf.StarStatus, DateWindow: vf.DateWindow}
	if vf.SearchQuery != "" {
		_, count, err := o.messages.SearchVirtualFolderConversations(folderIDs, vf.SearchQuery, 0, 0, criteria)
		return count, err
	}
	return o.messages.CountVirtualFolderConversations(folderIDs, criteria)
}

func (r *SyncRuntime) CreateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := r.virtualFolders.Create(vf); err != nil {
		return err
	}
	err := r.refreshVirtualFolderCache(vf.ID)
	if err == nil {
		r.publishChange(v1.EventViewsChanged, "", "")
	}
	return err
}

func (r *SyncRuntime) UpdateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := r.virtualFolders.Update(vf); err != nil {
		return err
	}
	err := r.refreshVirtualFolderCache(vf.ID)
	if err == nil {
		r.publishChange(v1.EventViewsChanged, "", "")
	}
	return err
}

func (r *SyncRuntime) DeleteVirtualFolder(id string) error {
	if r == nil || r.virtualFolders == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := r.virtualFolders.Delete(id); err != nil {
		return err
	}
	r.publishChange(v1.EventViewsChanged, "", "")
	return nil
}

func (r *SyncRuntime) ReorderVirtualFolders(ids []string) error {
	if r == nil || r.virtualFolders == nil {
		return errors.New("always-on runtime is not open")
	}
	if err := r.virtualFolders.Reorder(ids); err != nil {
		return err
	}
	r.publishChange(v1.EventViewsChanged, "", "")
	return nil
}

func (r *SyncRuntime) RefreshVirtualFolderCache(id string) error {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return errors.New("always-on runtime is not open")
	}
	err := r.refreshVirtualFolderCache(id)
	if err == nil {
		r.publishChange(v1.EventViewsChanged, "", "")
	}
	return err
}

func (r *SyncRuntime) GetCachedVirtualFolderView(ctx context.Context, id string) (*v1.CachedVirtualFolderView, error) {
	if r == nil || r.virtualFolders == nil || r.messages == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	vf, err := r.virtualFolders.Get(id)
	if err != nil {
		return nil, fmt.Errorf("load virtual folder for cached view: %w", err)
	}
	if vf == nil {
		return nil, fmt.Errorf("%w: %s", v1.ErrCachedVirtualFolderNotFound, id)
	}
	refs, refreshedAt, found, err := r.virtualFolders.LoadCache(id)
	if err != nil {
		return nil, err
	}
	if !found {
		if err := r.refreshVirtualFolderCacheContext(ctx, id); err != nil {
			return nil, err
		}
		refs, refreshedAt, found, err = r.virtualFolders.LoadCache(id)
		if err != nil {
			return nil, err
		}
	}
	conversationRefs := make([]message.ConversationRef, 0, len(refs))
	for _, ref := range refs {
		conversationRefs = append(conversationRefs, message.ConversationRef{AccountID: ref.AccountID, ConversationID: ref.ConversationID})
	}
	conversations, err := r.messages.ListConversationsByRefs(conversationRefs)
	if err != nil {
		return nil, err
	}
	return &v1.CachedVirtualFolderView{
		VirtualFolderID: id,
		Conversations:   conversations,
		RefreshedAt:     refreshedAt,
		HasCache:        found,
	}, nil
}

func (r *SyncRuntime) GetVirtualFolderUnreadCounts() (map[string]int, error) {
	if r == nil || r.virtualFolders == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	return r.virtualFolders.UnreadCounts()
}

var errProviderSearchCache = errors.New("saved search membership is maintained by provider search")

func (r *SyncRuntime) refreshVirtualFolderCache(id string) error {
	entries, err := r.virtualFolderCacheEntries(id)
	if errors.Is(err, errProviderSearchCache) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.virtualFolders.SaveCache(id, entries)
}

// refreshVirtualFolderCacheContext serves detached cache-route work: no
// retry loop runs, and the cache write's lock wait is bounded by
// SaveCacheContext's lowered busy_timeout - the driver ignores context
// cancellation during the busy wait - so an abandoned refresh frees its
// pool connection within that bound.
func (r *SyncRuntime) refreshVirtualFolderCacheContext(ctx context.Context, id string) error {
	entries, err := r.virtualFolderCacheEntries(id)
	if errors.Is(err, errProviderSearchCache) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.virtualFolders.SaveCacheContext(ctx, id, entries)
}

func (r *SyncRuntime) virtualFolderCacheEntries(id string) ([]virtualfolder.CachedConversation, error) {
	vf, err := r.virtualFolders.Get(id)
	if err != nil {
		return nil, fmt.Errorf("load virtual folder for cache refresh: %w", err)
	}
	if vf == nil {
		return nil, fmt.Errorf("virtual folder not found: %s", id)
	}
	folderIDs, err := r.virtualFolders.QueryFolderIDs(vf)
	if err != nil {
		return nil, fmt.Errorf("resolve virtual folder members: %w", err)
	}
	if vf.SearchQuery != "" {
		return nil, errProviderSearchCache
	}

	refs, err := r.messages.ListVirtualFolderConversationIDs(folderIDs, vf.SearchQuery, 200, message.VirtualFolderCriteria{
		ReadStatus: vf.ReadStatus, ReplyStatus: vf.ReplyStatus, StarStatus: vf.StarStatus, DateWindow: vf.DateWindow,
	})
	if err != nil {
		return nil, fmt.Errorf("query virtual folder cache members: %w", err)
	}
	entries := make([]virtualfolder.CachedConversation, 0, len(refs))
	for _, ref := range refs {
		entries = append(entries, virtualfolder.CachedConversation{AccountID: ref.AccountID, ConversationID: ref.ConversationID})
	}
	return entries, nil
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
	err = r.syncEngine.SyncFolders(ctx, accountID)
	if err == nil {
		r.publishChange(v1.EventFoldersChanged, accountID, "")
	}
	return err
}

// SetFolderSubscription applies provider subscription state and updates the helper cache.
func (r *SyncRuntime) SetFolderSubscription(ctx context.Context, accountID, folderID string, subscribed bool) error {
	defer r.publishChange(v1.EventFoldersChanged, accountID, "")
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
	defer r.publishChange(v1.EventFoldersChanged, accountID, "")
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
	actionReceipts := newActionReceiptLedger(r.db)
	r.actionReceiptRecoveryOnce.Do(func() {
		r.actionReceiptRecoveryErr = actionReceipts.RecoverInterrupted(context.Background())
	})
	if r.actionReceiptRecoveryErr != nil {
		return nil, fmt.Errorf("recover action receipts: %w", r.actionReceiptRecoveryErr)
	}
	operations := &catalogReadOperations{
		accounts:       r.accounts,
		folders:        r.folders,
		virtualFolders: r.virtualFolders,
		messages:       r.messages,
	}
	readOperations := &mailboxReadOperations{runtime: r}
	mailboxActions := &mailboxActionOperations{runtime: r}
	deps := &mailboxRouteDeps{
		catalog:            operations,
		cache:              r,
		unreadCounts:       r,
		accountMutations:   r,
		accountLedger:      newAccountMutationLedger(r.db),
		folders:            r,
		folderVisibility:   r,
		virtualFolders:     r,
		filterRules:        r,
		filterJobs:         r,
		drafts:             r,
		replyLater:         r,
		triage:             r,
		triageMutations:    r,
		headers:            operations,
		conversationList:   operations,
		conversationCount:  operations,
		conversationSearch: operations,
		bootstrap:          workspaceBootstrapOperations{ConversationListOperations: operations, ConversationListKeysetOperations: operations, catalog: operations, unreadCounts: r},
		search:             readOperations,
		messageRead:        readOperations,
		actionTarget:       mailboxActions,
		actions:            mailboxActions,
		receipts:           actionReceipts,
		undoStore:          actionReceipts,
		undoOps:            mailboxActions,
		eventStore:         actionReceipts,
		keyDigest:          func(key string) string { return actionReceipts.digestKey(key, r.accountMutationFingerprint) },
		grantClientID:      grantClientIDFromContext,
		fingerprint:        r.accountMutationFingerprint,
	}
	// Typed nil guard: a nil *mailboxevents.Broker assigned to the interface
	// field would read non-nil and mount the stream with no broker.
	if r.events != nil {
		deps.events = r.events
		deps.grantChecker = r.grantChecker
	}
	// The remote contacts surface shares this authenticated /v1 router; without
	// this mount every desktop contacts call in remote service mode 404s.
	if r.contacts != nil {
		deps.contacts = &contactsOperations{runtime: r.contacts}
		deps.contactsLedger = newContactsMutationLedger(r.db)
	}
	if r.calendar != nil {
		deps.calendar = r.calendar
	}
	mux := http.NewServeMux()
	mountMailboxV1Routes(mux, deps)
	return mux, nil
}

// grantClientIDFromContext reads the authenticated client ID the automation
// middleware stored on the request context.
func grantClientIDFromContext(request *http.Request) string {
	grant := automation.GetClientGrant(request.Context())
	if grant == nil {
		return ""
	}
	return grant.ClientID
}

// mailboxRouteDeps carries every operation the mailbox v1 route surface
// needs. MailboxReadAPIHandler fills it from the live runtime; the
// route-classification test fills it with stubs. Both mount through
// mountMailboxV1Routes, so a route added in one place is mounted in the
// other by construction and cannot leave the classification mirror stale.
type mailboxRouteDeps struct {
	catalog            v1.CatalogOperations
	cache              v1.CachedVirtualFolderOperations
	unreadCounts       v1.VirtualFolderUnreadCountOperations
	accountMutations   v1.AccountMutationOperations
	accountLedger      v1.AccountMutationLedger
	folders            v1.FolderMutationOperations
	folderVisibility   v1.FolderVisibilityOperations
	virtualFolders     v1.VirtualFolderMutationOperations
	filterRules        v1.FilterRuleOperations
	filterJobs         v1.FilterJobInspectionOperations
	drafts             v1.DraftOperations
	replyLater         v1.ReplyLaterHTTPOperations
	triage             v1.TriageOperations
	triageMutations    v1.TriageMutationOperations
	headers            v1.MessageHeaderOperations
	conversationList   v1.ConversationListOperations
	conversationCount  v1.ConversationCountOperations
	conversationSearch v1.ConversationSearchOperations
	bootstrap          v1.WorkspaceBootstrapOperations
	search             v1.SearchOperations
	messageRead        v1.MessageReadOperations
	actionTarget       operations.MessageLookup
	actions            v1.ActionOperations
	receipts           v1.ActionReceiptStore
	undoStore          v1.ActionUndoStore
	undoOps            v1.ActionUndoOperations
	eventStore         v1.ActionEventStore
	keyDigest          func(string) string
	grantClientID      func(*http.Request) string
	fingerprint        func([]byte) []byte
	contacts           v1.ContactsOperations
	contactsLedger     v1.ContactsMutationLedger
	calendar           v1.CalendarReadOperations
	events             v1.MailboxEventSource
	grantChecker       v1.GrantRevocationChecker
}

// mountMailboxV1Routes mounts the entire mailbox v1 route surface. Every
// route here is classified for read coalescing (PERF-SC-002) in
// route_classification_test.go, which mounts through this same function; a
// new route fails that test until it is classified.
func mountMailboxV1Routes(mux v1.RouteMux, d *mailboxRouteDeps) {
	requireCap := automation.RequireGrantCapability
	v1.MountCatalogRoutes(mux, d.catalog, requireCap)
	v1.MountVirtualFolderCacheRoute(mux, d.cache, requireCap)
	v1.MountVirtualFolderUnreadCountRoute(mux, d.unreadCounts, requireCap)
	v1.NewAccountMutationHandler(d.accountMutations, d.accountLedger, d.fingerprint).MountRoutes(mux, requireCap)
	v1.MountFolderMutationRoutes(mux, d.folders, requireCap)
	v1.MountFolderVisibilityRoutes(mux, d.folderVisibility, requireCap)
	v1.MountVirtualFolderMutationRoutes(mux, d.virtualFolders, requireCap)
	v1.MountFilterRuleRoutes(mux, d.filterRules, requireCap)
	v1.MountFilterJobRoutes(mux, d.filterJobs, requireCap)
	v1.NewDraftsHandler(d.drafts, d.fingerprint).MountRoutes(mux, requireCap)
	v1.MountReplyLaterRoutes(mux, d.replyLater, requireCap)
	v1.MountTriageRoutes(mux, d.triage, requireCap)
	v1.NewTriageMutationsHandler(d.triageMutations, d.receipts, d.keyDigest).MountRoutes(mux, requireCap)
	v1.MountMessageHeaderRoutes(mux, d.headers, requireCap)
	v1.MountConversationListRoute(mux, d.conversationList, requireCap)
	v1.MountConversationCountRoute(mux, d.conversationCount, requireCap)
	v1.MountConversationSearchRoute(mux, d.conversationSearch, requireCap)
	v1.MountWorkspaceBootstrapRoute(mux, d.bootstrap, requireCap)
	v1.MountSearchRoute(mux, d.search, requireCap)
	v1.MountThreadBodyRoutes(mux, d.messageRead, requireCap)
	v1.MountActionTargetRoute(mux, d.actionTarget, requireCap)
	v1.NewActionsHandlerWithReceiptStore(d.actions, d.receipts, d.keyDigest).MountRoutes(mux, requireCap)
	v1.MountActionUndoRoutes(mux, d.undoStore, d.undoOps, d.keyDigest, requireCap)
	v1.MountActionEventRoutes(mux, d.eventStore, d.grantClientID, requireCap)
	// The event stream mounts when the runtime has a broker; coalescing it
	// would buffer the stream, so it is classified non-coalescible.
	if d.events != nil {
		v1.MountEventsRoute(mux, d.events, d.grantChecker, d.grantClientID, requireCap)
	}
	if d.contacts != nil {
		v1.MountContactsRoutes(mux, d.contacts, d.contactsLedger, d.fingerprint, requireCap)
	}
	if d.calendar != nil {
		v1.MountCalendarReadRoutes(mux, d.calendar, requireCap)
	}
}

// workspaceBootstrapOperations adapts the runtime's read services to the
// composed interface behind POST /v1/workspace/bootstrap: the conversation
// list and catalog reads come from catalogReadOperations, saved-view unread
// counts from the runtime itself (as in MountVirtualFolderUnreadCountRoute).
type workspaceBootstrapOperations struct {
	v1.ConversationListKeysetOperations
	v1.ConversationListOperations
	catalog      v1.CatalogOperations
	unreadCounts v1.VirtualFolderUnreadCountOperations
}

func (w workspaceBootstrapOperations) GetAccounts() ([]*account.Account, error) {
	return w.catalog.GetAccounts()
}

func (w workspaceBootstrapOperations) GetFolders(accountID string) ([]*folder.Folder, error) {
	return w.catalog.GetFolders(accountID)
}

func (w workspaceBootstrapOperations) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) {
	return w.catalog.GetVirtualFolders()
}

func (w workspaceBootstrapOperations) GetVirtualFolderUnreadCounts() (map[string]int, error) {
	return w.unreadCounts.GetVirtualFolderUnreadCounts()
}

// PreviewFilterRule counts stored messages without changing mail or saving the rule.
func (r *SyncRuntime) PreviewFilterRule(rule *mailfilter.Rule) (int, error) {
	return r.rules.CountMatching(rule)
}

// GetMessageListID returns a captured List-ID, fetching the message header if needed.
func (r *SyncRuntime) GetMessageListID(id string) (string, error) {
	value, err := r.messages.GetListID(id)
	if err != nil || value != "" {
		return value, err
	}
	messages, err := r.messages.GetByIDs([]string{id})
	if err != nil {
		return "", err
	}
	if len(messages) != 1 {
		return "", errors.New("message not found")
	}
	m := messages[0]
	if r.syncEngine == nil {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, err := r.syncEngine.FetchRawMessage(ctx, m.AccountID, m.FolderID, m.UID)
	if err != nil {
		return "", err
	}
	value = syncengine.ExtractListID(raw)
	if err := r.messages.SetListID(id, value); err != nil {
		return "", err
	}
	return value, nil
}

// ApplyFilterRuleToExisting queues explicit application using a snapshot of the saved rule.
func (r *SyncRuntime) ApplyFilterRuleToExisting(id string) (int, error) {
	rule, err := r.rules.Get(id)
	if err != nil {
		return 0, err
	}
	if rule == nil {
		return 0, errors.New("filter rule not found")
	}
	if err := mailfilter.ValidateRule(rule, r.accounts, r.folders); err != nil {
		return 0, err
	}
	if rule.DestFolderID != "" {
		d, e := r.folders.Get(rule.DestFolderID)
		if e != nil {
			return 0, e
		}
		if d == nil || d.Hidden {
			return 0, errors.New("filter destination unavailable or hidden")
		}
	}
	ids, err := r.rules.MatchingMessageIDs(rule, 501)
	if err != nil {
		return 0, err
	}
	if len(ids) > 500 {
		return 0, errors.New("more than 500 messages match; narrow the rule before applying to existing mail")
	}
	return r.applyExistingFilter(rule, ids)
}

// applyExistingFilter records explicit application in the durable queue. Provider
// work must outlive HTTP request timeouts without encouraging duplicate retries.
func (r *SyncRuntime) applyExistingFilter(rule *mailfilter.Rule, ids []string) (int, error) {
	messages, err := r.messages.GetByIDs(ids)
	if err != nil {
		return 0, err
	}
	if len(messages) == 0 {
		return 0, nil
	}
	if err := r.filterJobs.EnqueueBatch(rule, rule.DestFolderID, messages, true); err != nil {
		return 0, err
	}

	select {
	case r.filterWake <- struct{}{}:
	default:
	}
	return len(messages), nil
}
