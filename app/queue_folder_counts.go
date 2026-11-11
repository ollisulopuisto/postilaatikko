package app

import "fmt"

// QueueFolderCounts reports, per account and folder, how many conversations in
// the Read Later and Reply Later queues have locally indexed messages there.
// The folder panel uses this to filter the folder tree by queue membership.
// Queue entries whose messages are not synced locally yet contribute nothing.
type QueueFolderCounts struct {
	// ReadLater maps account ID -> folder ID -> queued conversation count.
	ReadLater map[string]map[string]int `json:"readLater"`
	// ReplyLater maps account ID -> folder ID -> queued conversation count.
	ReplyLater map[string]map[string]int `json:"replyLater"`
}

// GetQueueFolderCounts returns per-folder membership counts for the Read
// Later and Reply Later queues. Queue membership comes from the same
// service-aware getters as the queue views, so a remote mailbox service's
// entries count too; only the folder resolution is local, through the message
// index.
func (a *App) GetQueueFolderCounts() (*QueueFolderCounts, error) {
	if a.messageStore == nil {
		return nil, fmt.Errorf("message store is not ready")
	}
	readLaterRefs, err := a.GetReadLaterQueue()
	if err != nil {
		return nil, fmt.Errorf("list Read Later queue: %w", err)
	}
	replyLaterRefs, err := a.GetReplyLaterQueue()
	if err != nil {
		return nil, fmt.Errorf("list Reply Later queue: %w", err)
	}
	readLaterByAccount := map[string][]string{}
	for _, ref := range readLaterRefs {
		readLaterByAccount[ref.AccountID] = append(readLaterByAccount[ref.AccountID], ref.ConversationID)
	}
	replyLaterByAccount := map[string][]string{}
	for _, ref := range replyLaterRefs {
		replyLaterByAccount[ref.AccountID] = append(replyLaterByAccount[ref.AccountID], ref.ConversationID)
	}
	readLater, err := a.messageStore.FolderCountsForConversations(readLaterByAccount)
	if err != nil {
		return nil, fmt.Errorf("count Read Later folders: %w", err)
	}
	replyLater, err := a.messageStore.FolderCountsForConversations(replyLaterByAccount)
	if err != nil {
		return nil, fmt.Errorf("count Reply Later folders: %w", err)
	}
	return &QueueFolderCounts{ReadLater: readLater, ReplyLater: replyLater}, nil
}
