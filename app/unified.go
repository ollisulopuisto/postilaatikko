package app

import (
	"errors"
	"fmt"

	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/triage"
	"github.com/ollisulopuisto/postilaatikko/internal/views"
)

// unifiedSpecialFolders resolves the real folders represented by a synthetic
// unified navigation type. GetSpecialFolder is deliberately the single source
// of truth: it honors an account's configured path before folder_type.
func (a *App) unifiedSpecialFolders(folderType string) ([]*folder.Folder, error) {
	if folderType == "triage" {
		return triage.DefaultFolderScope(a.accountStore, a.folderStore)
	}
	if folderType == "replyLater" || folderType == "readLater" || folderType == "snoozed" {
		accounts, err := a.accountStore.List()
		if err != nil {
			return nil, err
		}
		folders := []*folder.Folder{}
		for _, account := range accounts {
			if !account.Enabled {
				continue
			}
			items, err := a.folderStore.List(account.ID)
			if err != nil {
				return nil, err
			}
			folders = append(folders, items...)
		}
		return folders, nil
	}
	typeValue := folder.Type(folderType)
	switch typeValue {
	case folder.TypeInbox, folder.TypeDrafts, folder.TypeSent, folder.TypeTrash, folder.TypeStarred, folder.TypeArchive, folder.TypeSpam, folder.TypeAll:
	default:
		return nil, fmt.Errorf("unsupported unified folder type: %s", folderType)
	}
	accounts, err := a.accountStore.List()
	if err != nil {
		return nil, err
	}
	resolved := make([]*folder.Folder, 0, len(accounts))
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		item, err := a.GetSpecialFolder(account.ID, typeValue)
		if err != nil {
			return nil, err
		}
		if item != nil {
			resolved = append(resolved, item)
		}
	}
	return resolved, nil
}

func (a *App) unifiedSpecialFolderIDs(folderType string) ([]string, error) {
	folders, err := a.unifiedSpecialFolders(folderType)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(folders))
	for _, item := range folders {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func (a *App) GetUnifiedFolderConversations(folderType string, offset, limit int, sortOrder, filterValue string) ([]*message.Conversation, error) {
	filterValue = replyLaterViewFilter(folderType, filterValue)
	if folderType == string(folder.TypeInbox) {
		conversations, handled, err := a.listServiceConversations(views.Scope{Kind: views.ScopeAllInboxes}, offset, limit, sortOrder, filterValue)
		if err != nil {
			return nil, err
		}
		if handled {
			go a.enqueueUnifiedInboxConversationPrefetch(conversations)
			return conversations, nil
		}
	}
	if a.remoteServiceMode {
		if !isRemoteUnifiedFolderType(folderType) {
			return nil, fmt.Errorf("unified %s conversations are not available from the remote mailbox service", folderType)
		}
		conversations, handled, err := a.listServiceConversations(views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: folderType}, offset, limit, sortOrder, filterValue)
		if err != nil {
			return nil, err
		}
		if !handled {
			return nil, errors.New("remote mailbox service is not connected")
		}
		return conversations, nil
	}
	if folderType == "readLater" || folderType == "replyLater" {
		if offset == 0 {
			a.syncGmailTriageQueues("read-later")
			a.syncGmailTriageQueues("reply-later")
		}
	}
	// Scopes the helper serves run its implementation in-process, so local
	// and service modes cannot drift. Snoozed and All Mail are desktop-only.
	if scope, ok := sharedUnifiedScope(folderType); ok {
		return a.listLocalConversations(scope, offset, limit, sortOrder, filterValue)
	}
	if folderType == "snoozed" {
		return a.messageStore.ListSnoozedConversations(offset, limit, sortOrder, filterValue)
	}
	ids, err := a.unifiedSpecialFolderIDs(folderType)
	if err != nil {
		return nil, err
	}
	return a.messageStore.ListConversationsByFolderIDs(ids, offset, limit, sortOrder, filterValue)
}

func (a *App) GetUnifiedFolderCount(folderType, filterValue string) (int, error) {
	filterValue = replyLaterViewFilter(folderType, filterValue)
	if folderType == string(folder.TypeInbox) {
		count, handled, err := a.serviceConversationCount(views.Scope{Kind: views.ScopeAllInboxes}, filterValue)
		if handled || err != nil {
			return count, err
		}
	}
	if a.remoteServiceMode {
		if !isRemoteUnifiedFolderType(folderType) {
			return 0, fmt.Errorf("unified %s count is not available from the remote mailbox service", folderType)
		}
		count, handled, err := a.serviceConversationCount(views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: folderType}, filterValue)
		if err != nil {
			return 0, err
		}
		if !handled {
			return 0, errors.New("remote mailbox service is not connected")
		}
		return count, nil
	}
	if scope, ok := sharedUnifiedScope(folderType); ok {
		return a.countLocalConversations(scope, filterValue)
	}
	if folderType == "snoozed" {
		return a.messageStore.CountSnoozedConversations(filterValue)
	}
	ids, err := a.unifiedSpecialFolderIDs(folderType)
	if err != nil {
		return 0, err
	}
	return a.messageStore.CountConversationsByFolderIDs(ids, filterValue)
}

func (a *App) SearchUnifiedFolder(folderType, query string, offset, limit int, filterValue string) ([]*message.ConversationSearchResult, error) {
	filterValue = replyLaterViewFilter(folderType, filterValue)
	if a.remoteServiceMode {
		page, err := a.searchRemoteUnifiedFolder(folderType, query, offset, limit, filterValue)
		if err != nil {
			return nil, err
		}
		return page.Results, nil
	}
	ids, err := a.unifiedSpecialFolderIDs(folderType)
	if err != nil {
		return nil, err
	}
	results, _, err := a.messageStore.SearchConversationsByFolderIDs(ids, query, offset, limit, filterValue)
	return results, err
}

func (a *App) GetSearchCountUnifiedFolder(folderType, query, filterValue string) (int, error) {
	filterValue = replyLaterViewFilter(folderType, filterValue)
	if a.remoteServiceMode {
		page, err := a.searchRemoteUnifiedFolder(folderType, query, 0, 1, filterValue)
		if err != nil {
			return 0, err
		}
		return page.TotalCount, nil
	}
	ids, err := a.unifiedSpecialFolderIDs(folderType)
	if err != nil {
		return 0, err
	}
	_, count, err := a.messageStore.SearchConversationsByFolderIDs(ids, query, 0, 0, filterValue)
	return count, err
}

// sharedUnifiedScope maps a unified navigation type to the v1 scope the
// helper serves. Local mode routes these through the helper's in-process
// implementation; the remote service accepts exactly the same set.
func sharedUnifiedScope(folderType string) (views.Scope, bool) {
	if folderType == string(folder.TypeInbox) {
		return views.Scope{Kind: views.ScopeAllInboxes}, true
	}
	if isRemoteUnifiedFolderType(folderType) {
		return views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: folderType}, true
	}
	return views.Scope{}, false
}

func isRemoteUnifiedFolderType(folderType string) bool {
	if folderType == "triage" || folderType == "readLater" || folderType == "replyLater" {
		return true
	}
	switch folder.Type(folderType) {
	case folder.TypeDrafts, folder.TypeSent, folder.TypeTrash, folder.TypeStarred, folder.TypeArchive, folder.TypeSpam:
		return true
	default:
		return false
	}
}

func (a *App) searchRemoteUnifiedFolder(folderType, query string, offset, limit int, filterValue string) (*v1.ConversationSearchPage, error) {
	if !isRemoteUnifiedFolderType(folderType) {
		return nil, fmt.Errorf("unified %s search is not available from the remote mailbox service", folderType)
	}
	if a.mailboxService == nil {
		return nil, errors.New("remote mailbox service is not connected")
	}
	ctx := a.actionContext()
	status := a.mailboxService.status(ctx)
	if !status.Connected {
		return nil, fmt.Errorf("remote mailbox service search unavailable: %s", status.Message)
	}
	return a.mailboxService.searchUnifiedFolder(ctx, v1.ConversationSearchRequest{
		Scope: views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: folderType},
		Query: query, Offset: offset, Limit: limit, Filter: filterValue,
	})
}

// SyncUnifiedFolder syncs each represented real folder. It never passes a
// synthetic navigation ID to the sync engine.
func (a *App) SyncUnifiedFolder(folderType string) error {
	if a.remoteServiceMode {
		return errors.New("mailbox sync is owned by the configured remote helper")
	}
	if folderType == "replyLater" || folderType == "readLater" || folderType == "snoozed" {
		if folderType == "snoozed" {
			return nil
		}
		queue := "reply-later"
		if folderType == "readLater" {
			queue = "read-later"
		}
		a.syncGmailTriageQueues(queue)
		return nil
	}
	folders, err := a.unifiedSpecialFolders(folderType)
	if err != nil {
		return err
	}
	for _, item := range folders {
		if err := a.SyncFolder(item.AccountID, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) EmptyUnifiedTrash() error {
	folders, err := a.unifiedSpecialFolders(string(folder.TypeTrash))
	if err != nil {
		return err
	}
	for _, item := range folders {
		if err := a.EmptyTrash(item.AccountID, item.ID); err != nil {
			return err
		}
	}
	return nil
}

// replyLaterViewFilter keeps queue membership applied when quick filters or
// local search further narrow the first-class queue view.
func replyLaterViewFilter(folderType, filter string) string {
	if folderType != "replyLater" && folderType != "readLater" {
		return filter
	}
	queueFilter := folderType
	if filter == "" || filter == queueFilter {
		return queueFilter
	}
	return queueFilter + "+" + filter
}
