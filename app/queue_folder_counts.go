package app

import "fmt"

// QueueFolderCounts reports, per account and folder, how many conversations in
// the app-local triage queues (Read Later, Reply Later) have locally indexed
// messages there. The folder panel uses this to filter the folder tree by
// queue membership. Queue entries whose messages are not synced locally yet
// contribute nothing.
type QueueFolderCounts struct {
	// ReadLater maps account ID -> folder ID -> queued conversation count.
	ReadLater map[string]map[string]int `json:"readLater"`
	// ReplyLater maps account ID -> folder ID -> queued conversation count.
	ReplyLater map[string]map[string]int `json:"replyLater"`
}

// GetQueueFolderCounts returns per-folder membership counts for the app-local
// Read Later and Reply Later queues.
func (a *App) GetQueueFolderCounts() (*QueueFolderCounts, error) {
	if a.triageStore == nil || a.replyLaterStore == nil {
		return nil, fmt.Errorf("triage queues are not ready")
	}
	readLater, err := a.triageStore.ReadLaterFolderCounts()
	if err != nil {
		return nil, err
	}
	replyLater, err := a.replyLaterStore.FolderCounts()
	if err != nil {
		return nil, err
	}
	return &QueueFolderCounts{ReadLater: readLater, ReplyLater: replyLater}, nil
}
