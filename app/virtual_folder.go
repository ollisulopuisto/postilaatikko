package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

// ============================================================================
// Virtual Folder API - Exposed to frontend via Wails bindings
// ============================================================================

// GetVirtualFolders returns all virtual folders
func (a *App) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) {
	return a.virtualFolderStore.List()
}

// GetVirtualFolderUnreadCounts returns unread conversation counts for all virtual folders.
func (a *App) GetVirtualFolderUnreadCounts() (map[string]int, error) {
	return a.virtualFolderStore.UnreadCounts()
}

// CreateVirtualFolder creates a new virtual folder
func (a *App) CreateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if vf == nil {
		return fmt.Errorf("virtual folder cannot be nil")
	}
	if err := a.virtualFolderStore.Create(vf); err != nil {
		return err
	}
	// Warm the index cache so the first open renders instantly.
	if err := a.refreshVirtualFolderCache(vf); err != nil {
		log := logging.WithComponent("virtualfolder")
		log.Warn().Err(err).Str("virtualFolderID", vf.ID).Msg("Failed to warm virtual-folder cache on create")
	}
	return nil
}

// UpdateVirtualFolder updates an existing virtual folder
func (a *App) UpdateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if vf == nil {
		return fmt.Errorf("virtual folder cannot be nil")
	}
	if vf.ID == "" {
		return fmt.Errorf("virtual folder ID cannot be empty")
	}
	if err := a.virtualFolderStore.Update(vf); err != nil {
		return err
	}
	// Folder set or criteria may have changed; rebuild the cached index.
	if err := a.refreshVirtualFolderCache(vf); err != nil {
		log := logging.WithComponent("virtualfolder")
		log.Warn().Err(err).Str("virtualFolderID", vf.ID).Msg("Failed to refresh virtual-folder cache on update")
	}
	return nil
}

// ReorderVirtualFolders saves the sidebar order of virtual folders.
func (a *App) ReorderVirtualFolders(ids []string) error {
	return a.virtualFolderStore.Reorder(ids)
}

// DeleteVirtualFolder deletes a virtual folder
func (a *App) DeleteVirtualFolder(id string) error {
	if id == "" {
		return fmt.Errorf("virtual folder ID cannot be empty")
	}
	return a.virtualFolderStore.Delete(id)
}

// GetVirtualFolderConversations returns conversations matching a virtual folder's criteria
func (a *App) GetVirtualFolderConversations(virtualFolderID string, offset, limit int, sortOrder string) ([]*message.Conversation, error) {
	if a.syncEngine != nil {
		result := []*message.Conversation{}
		cursor := ""
		if limit <= 0 {
			limit = 50
		}
		for pages := 0; pages < 100; pages++ {
			// SearchMail applies the folder's saved search query itself.
			page, err := a.SearchMail(MailSearchRequest{VirtualFolderID: virtualFolderID, Cursor: cursor, Limit: 100})
			if err != nil {
				return nil, err
			}
			if len(page.Issues) > 0 {
				return nil, fmt.Errorf("virtual-folder account unavailable: %s", page.Issues[0].Message)
			}
			result = append(result, page.Conversations...)
			cursor = page.Cursor
			if cursor == "" || len(result) >= offset+limit {
				break
			}
		}
		if sortOrder == "oldest" {
			sort.Slice(result, func(i, j int) bool { return result[i].LatestDate.Before(result[j].LatestDate) })
		}
		if offset >= len(result) {
			return []*message.Conversation{}, nil
		}
		end := offset + limit
		if end > len(result) {
			end = len(result)
		}
		return result[offset:end], nil
	}

	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return nil, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}

	folderIDs, err := a.virtualFolderStore.QueryFolderIDs(vf)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve virtual folder members: %w", err)
	}
	if len(folderIDs) == 0 {
		return []*message.Conversation{}, nil
	}
	if vf.SearchQuery != "" {
		// A saved search defines membership: list via the text-search path so the
		// stored query and the saved criteria both apply.
		results, _, err := a.messageStore.SearchVirtualFolderConversations(folderIDs, vf.SearchQuery, offset, limit, virtualFolderMessageCriteria(vf))
		if err != nil {
			return nil, err
		}
		conversations := make([]*message.Conversation, 0, len(results))
		for _, result := range results {
			conversations = append(conversations, &result.Conversation)
		}
		return conversations, nil
	}
	return a.messageStore.ListVirtualFolderConversations(folderIDs, offset, limit, sortOrder, virtualFolderMessageCriteria(vf))
}

// GetVirtualFolderCount returns the count of conversations matching a virtual folder's criteria.
func (a *App) GetVirtualFolderCount(virtualFolderID string) (int, error) {
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return 0, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return 0, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}

	folderIDs, err := a.virtualFolderStore.QueryFolderIDs(vf)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve virtual folder members: %w", err)
	}
	if len(folderIDs) == 0 {
		return 0, nil
	}
	if vf.SearchQuery != "" {
		_, count, err := a.messageStore.SearchVirtualFolderConversations(folderIDs, vf.SearchQuery, 0, 0, virtualFolderMessageCriteria(vf))
		return count, err
	}
	return a.messageStore.CountVirtualFolderConversations(folderIDs, virtualFolderMessageCriteria(vf))
}

// SearchVirtualFolder searches messages in a virtual folder's member folders.
func (a *App) SearchVirtualFolder(virtualFolderID, query string, offset, limit int, _ string) ([]*message.ConversationSearchResult, error) {
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return nil, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}
	folderIDs, err := a.virtualFolderStore.QueryFolderIDs(vf)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve virtual folder members: %w", err)
	}
	query = combineVirtualFolderQueries(vf.SearchQuery, query)
	results, _, err := a.messageStore.SearchVirtualFolderConversations(folderIDs, query, offset, limit, virtualFolderMessageCriteria(vf))
	return results, err
}

// GetVirtualFolderSearchCount returns the matching message-search count.
func (a *App) GetVirtualFolderSearchCount(virtualFolderID, query, _ string) (int, error) {
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return 0, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return 0, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}
	folderIDs, err := a.virtualFolderStore.QueryFolderIDs(vf)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve virtual folder members: %w", err)
	}
	query = combineVirtualFolderQueries(vf.SearchQuery, query)
	_, count, err := a.messageStore.SearchVirtualFolderConversations(folderIDs, query, 0, 0, virtualFolderMessageCriteria(vf))
	return count, err
}

func virtualFolderMessageCriteria(vf *virtualfolder.VirtualFolder) message.VirtualFolderCriteria {
	return message.VirtualFolderCriteria{
		ReadStatus:  vf.ReadStatus,
		ReplyStatus: vf.ReplyStatus,
		StarStatus:  vf.StarStatus,
		DateWindow:  vf.DateWindow,
	}
}

// combineVirtualFolderQueries ANDs a saved search query with a typed in-folder
// search. FTS5 treats space-separated terms as AND, and both sides go through
// the same term preparation in the message store.
func combineVirtualFolderQueries(stored, typed string) string {
	return strings.TrimSpace(strings.TrimSpace(stored) + " " + strings.TrimSpace(typed))
}

// virtualFolderCacheLimit caps how many conversations one cached index holds.
const virtualFolderCacheLimit = 200

// VirtualFolderCachedView is a virtual folder's cached conversation index for
// instant open. RefreshedAt tells the UI how stale the membership may be.
type VirtualFolderCachedView struct {
	VirtualFolderID string                  `json:"virtualFolderId"`
	Conversations   []*message.Conversation `json:"conversations"`
	RefreshedAt     time.Time               `json:"refreshedAt"`
	HasCache        bool                    `json:"hasCache"`
}

// GetVirtualFolderCachedView returns a virtual folder's cached conversation
// index, computing and storing it on first use. Row state (unread, stars,
// snippets) is hydrated live; only membership and order come from the cache.
func (a *App) GetVirtualFolderCachedView(virtualFolderID string) (*VirtualFolderCachedView, error) {
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return nil, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}
	refs, refreshedAt, hasCache, err := a.virtualFolderStore.LoadCache(virtualFolderID)
	if err != nil {
		return nil, err
	}
	if !hasCache {
		if err := a.refreshVirtualFolderCache(vf); err != nil {
			return nil, err
		}
		if refs, refreshedAt, _, err = a.virtualFolderStore.LoadCache(virtualFolderID); err != nil {
			return nil, err
		}
	}
	msgRefs := make([]message.ConversationRef, 0, len(refs))
	for _, ref := range refs {
		msgRefs = append(msgRefs, message.ConversationRef{AccountID: ref.AccountID, ConversationID: ref.ConversationID})
	}
	conversations, err := a.messageStore.ListConversationsByRefs(msgRefs)
	if err != nil {
		return nil, err
	}
	return &VirtualFolderCachedView{
		VirtualFolderID: virtualFolderID,
		Conversations:   conversations,
		RefreshedAt:     refreshedAt,
		HasCache:        true,
	}, nil
}

// RefreshVirtualFolderCache recomputes one virtual folder's cached index and
// notifies the frontend. Used by the background refresher and available for a
// manual refresh.
func (a *App) RefreshVirtualFolderCache(virtualFolderID string) error {
	a.invalidateViewCounts()
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}
	return a.refreshVirtualFolderCache(vf)
}

// refreshVirtualFolderCache recomputes and stores the cached index for one
// virtual folder, then emits virtualfolder:cacheUpdated so an open view can
// pick up the new membership.
func (a *App) refreshVirtualFolderCache(vf *virtualfolder.VirtualFolder) error {
	folderIDs, err := a.virtualFolderStore.QueryFolderIDs(vf)
	if err != nil {
		return fmt.Errorf("failed to resolve virtual folder members: %w", err)
	}
	refs := []message.ConversationRef{}
	if len(folderIDs) > 0 {
		refs, err = a.messageStore.ListVirtualFolderConversationIDs(folderIDs, vf.SearchQuery, virtualFolderCacheLimit, virtualFolderMessageCriteria(vf))
		if err != nil {
			return err
		}
	}
	entries := make([]virtualfolder.CachedConversation, 0, len(refs))
	for _, ref := range refs {
		entries = append(entries, virtualfolder.CachedConversation{AccountID: ref.AccountID, ConversationID: ref.ConversationID})
	}
	if err := a.virtualFolderStore.SaveCache(vf.ID, entries); err != nil {
		return err
	}
	if a.ctx != nil {
		a.emitRuntimeEvent("virtualfolder:cacheUpdated", map[string]interface{}{
			"virtualFolderId": vf.ID,
		})
	}
	return nil
}

// virtualFolderCacheRefreshInterval is how often cached virtual-folder indexes
// are recomputed in the background.
const virtualFolderCacheRefreshInterval = 5 * time.Minute

// startVirtualFolderCacheRefresher periodically refreshes every virtual
// folder's cached index so saved views open instantly without going stale.
// Failures on one folder are logged and do not stop the others.
func (a *App) startVirtualFolderCacheRefresher(ctx context.Context) {
	log := logging.WithComponent("virtualfolder")
	ticker := time.NewTicker(virtualFolderCacheRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			folders, err := a.virtualFolderStore.List()
			if err != nil {
				log.Warn().Err(err).Msg("Failed to list virtual folders for cache refresh")
				continue
			}
			for _, vf := range folders {
				if err := a.refreshVirtualFolderCache(vf); err != nil {
					log.Warn().Err(err).Str("virtualFolderID", vf.ID).Msg("Failed to refresh virtual-folder cache")
				}
			}
		}
	}
}
