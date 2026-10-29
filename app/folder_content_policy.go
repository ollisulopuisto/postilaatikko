package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/message"
)

const maxQueuedFolderBodyPrefetch = 128

type folderBodyPrefetchQueue struct {
	accountID string
	folderID  string
	ctx       context.Context
	cancel    context.CancelFunc
	ids       []string
	pending   map[string]bool
}

// FolderContentPolicy reports the effective body-prefetch setting and where it
// came from. Attachments are never included in this policy.
type FolderContentPolicy struct {
	AccountID    string `json:"accountId"`
	FolderID     string `json:"folderId"`
	BodyPrefetch bool   `json:"bodyPrefetch"`
	Explicit     bool   `json:"explicit"`
	Source       string `json:"source"`
}

// GetFolderContentPolicy returns the explicit choice or the sidebar-derived
// default for a folder. Inbox, Starred and pinned folders prefetch new message
// bodies; other folders keep header-only sync behavior.
func (a *App) GetFolderContentPolicy(accountID, folderID string) (*FolderContentPolicy, error) {
	if a.contentPolicyStore == nil {
		return nil, fmt.Errorf("folder content policy store is not initialized")
	}
	f, err := a.folderStore.Get(folderID)
	if err != nil {
		return nil, err
	}
	if f == nil || f.AccountID != accountID {
		return nil, fmt.Errorf("folder does not belong to account")
	}
	policy := &FolderContentPolicy{AccountID: accountID, FolderID: folderID}
	override, err := a.contentPolicyStore.Get(accountID, folderID)
	if err != nil {
		return nil, err
	}
	if override != nil {
		policy.BodyPrefetch = override.Enabled
		policy.Explicit = true
		policy.Source = "user"
		return policy, nil
	}
	if f.Type == folder.TypeInbox {
		policy.BodyPrefetch = true
		policy.Source = "inbox"
		return policy, nil
	}
	if f.Type == folder.TypeStarred {
		policy.BodyPrefetch = true
		policy.Source = "starred"
		return policy, nil
	}
	state, err := a.appStateStore.GetUIState()
	if err != nil {
		return nil, fmt.Errorf("read sidebar pins: %w", err)
	}
	for _, pin := range state.PinnedFolders {
		if pin.AccountID == accountID && pin.FolderID == folderID {
			policy.BodyPrefetch = true
			policy.Source = "sidebar"
			return policy, nil
		}
	}
	policy.Source = "headers-only"
	return policy, nil
}

// SetFolderContentPolicy stores an explicit choice. mode must be "enabled",
// "disabled", or "inherit". Choosing "inherit" restores the current default.
// Enabling a folder only affects new messages; it does not backfill history.
func (a *App) SetFolderContentPolicy(accountID, folderID, mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "enabled" && mode != "disabled" && mode != "inherit" {
		return fmt.Errorf("folder content policy must be enabled, disabled, or inherit")
	}
	f, err := a.folderStore.Get(folderID)
	if err != nil {
		return err
	}
	if f == nil || f.AccountID != accountID {
		return fmt.Errorf("folder does not belong to account")
	}
	if mode == "inherit" {
		err = a.contentPolicyStore.Reset(accountID, folderID)
	} else {
		err = a.contentPolicyStore.Set(accountID, folderID, mode == "enabled")
	}
	if err != nil {
		return err
	}
	if mode == "disabled" {
		a.cancelFolderBodyPrefetch(accountID, folderID)
	}
	return nil
}

func (a *App) handleNewSyncedMessage(accountID, folderID string, msg *message.Message) {
	a.handleNewMessageForFilters(accountID, folderID, msg)
	if msg == nil {
		return
	}
	a.enqueueFolderBodyPrefetch(accountID, folderID, msg.ID)
}

func (a *App) enqueueFolderBodyPrefetch(accountID, folderID, messageID string) {
	if messageID == "" || a.syncEngine == nil || a.ctx == nil {
		return
	}
	policy, err := a.GetFolderContentPolicy(accountID, folderID)
	if err != nil || !policy.BodyPrefetch {
		return
	}
	key := accountID + ":" + folderID
	a.bodyPrefetchMu.Lock()
	if a.bodyPrefetchQueues == nil {
		a.bodyPrefetchQueues = make(map[string]*folderBodyPrefetchQueue)
	}
	if a.bodyPrefetchSlots == nil {
		a.bodyPrefetchSlots = make(chan struct{}, 2)
	}
	queue := a.bodyPrefetchQueues[key]
	start := queue == nil
	if start {
		ctx, cancel := context.WithCancel(a.ctx)
		queue = &folderBodyPrefetchQueue{accountID: accountID, folderID: folderID, ctx: ctx, cancel: cancel, pending: make(map[string]bool)}
		a.bodyPrefetchQueues[key] = queue
	}
	if queue.pending[messageID] {
		a.bodyPrefetchMu.Unlock()
		return
	}
	if len(queue.pending) >= maxQueuedFolderBodyPrefetch {
		a.bodyPrefetchMu.Unlock()
		log := logging.WithComponent("app.body-prefetch")
		log.Warn().Str("folder", folderID).Msg("Body prefetch queue full; leaving message on the server until opened")
		return
	}
	queue.pending[messageID] = true
	queue.ids = append(queue.ids, messageID)
	a.bodyPrefetchMu.Unlock()
	if start {
		go a.runFolderBodyPrefetch(key, queue)
	}
}

func (a *App) runFolderBodyPrefetch(key string, queue *folderBodyPrefetchQueue) {
	defer queue.cancel()
	log := logging.WithComponent("app.body-prefetch")
	for {
		a.bodyPrefetchMu.Lock()
		if len(queue.ids) == 0 {
			if a.bodyPrefetchQueues[key] == queue {
				delete(a.bodyPrefetchQueues, key)
			}
			a.bodyPrefetchMu.Unlock()
			return
		}
		messageID := queue.ids[0]
		queue.ids = queue.ids[1:]
		a.bodyPrefetchMu.Unlock()

		select {
		case a.bodyPrefetchSlots <- struct{}{}:
		case <-queue.ctx.Done():
			return
		}
		policy, err := a.GetFolderContentPolicy(queue.accountID, queue.folderID)
		if err == nil && policy.BodyPrefetch && queue.ctx.Err() == nil {
			msg, getErr := a.messageStore.Get(messageID)
			if getErr == nil && msg != nil && msg.AccountID == queue.accountID && !msg.BodyFetched {
				if _, fetchErr := a.syncEngine.FetchMessageBody(queue.ctx, queue.accountID, messageID); fetchErr != nil && queue.ctx.Err() == nil {
					log.Warn().Err(fetchErr).Str("folder", queue.folderID).Msg("Automatic body prefetch failed; message remains fetchable on demand")
				}
			}
		}
		<-a.bodyPrefetchSlots
		a.bodyPrefetchMu.Lock()
		delete(queue.pending, messageID)
		a.bodyPrefetchMu.Unlock()
	}
}

func (a *App) cancelFolderBodyPrefetch(accountID, folderID string) {
	key := accountID + ":" + folderID
	a.bodyPrefetchMu.Lock()
	queue := a.bodyPrefetchQueues[key]
	if queue != nil {
		delete(a.bodyPrefetchQueues, key)
		queue.cancel()
	}
	a.bodyPrefetchMu.Unlock()
}
