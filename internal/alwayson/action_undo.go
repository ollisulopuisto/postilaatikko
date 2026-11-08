package alwayson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/message"
)

type mailboxActionUndoSnapshot struct {
	Action  v1.ActionKind                    `json:"action"`
	Entries []mailboxActionUndoSnapshotEntry `json:"entries"`
}

type mailboxActionUndoSnapshotEntry struct {
	AccountID string `json:"accountId"`
	MessageID string `json:"messageId"`
	FolderID  string `json:"folderId"`
	IsRead    bool   `json:"isRead"`
	IsStarred bool   `json:"isStarred"`
}

func (o *mailboxActionOperations) CaptureActionUndo(ctx context.Context, action v1.ActionKind, ids []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return nil, err
	}
	snapshot := mailboxActionUndoSnapshot{Action: action, Entries: make([]mailboxActionUndoSnapshotEntry, 0, len(messages))}
	for _, msg := range messages {
		snapshot.Entries = append(snapshot.Entries, mailboxActionUndoSnapshotEntry{
			AccountID: msg.AccountID,
			MessageID: msg.ID,
			FolderID:  msg.FolderID,
			IsRead:    msg.IsRead,
			IsStarred: msg.IsStarred,
		})
	}
	return json.Marshal(snapshot)
}

func (o *mailboxActionOperations) UndoAction(ctx context.Context, action v1.ActionKind, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var snapshot mailboxActionUndoSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return fmt.Errorf("decode action undo snapshot: %w", err)
	}
	if snapshot.Action != action || len(snapshot.Entries) == 0 {
		return errors.New("action undo snapshot does not match operation")
	}
	ids := make([]string, 0, len(snapshot.Entries))
	byID := make(map[string]mailboxActionUndoSnapshotEntry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.AccountID == "" || entry.MessageID == "" || entry.FolderID == "" {
			return errors.New("action undo snapshot has incomplete identity")
		}
		if _, exists := byID[entry.MessageID]; exists {
			return errors.New("action undo snapshot has duplicate messages")
		}
		byID[entry.MessageID] = entry
		ids = append(ids, entry.MessageID)
	}
	current, err := o.loadActionMessages(ids)
	if err != nil {
		return err
	}
	for _, msg := range current {
		entry := byID[msg.ID]
		if entry.AccountID != msg.AccountID {
			return errors.New("action undo target account no longer matches")
		}
	}

	switch action {
	case v1.ActionArchive, v1.ActionTrash, v1.ActionMove:
		byFolder := make(map[string][]*message.Message)
		folderAccounts := make(map[string]string)
		for _, msg := range current {
			entry := byID[msg.ID]
			byFolder[entry.FolderID] = append(byFolder[entry.FolderID], msg)
			folderAccounts[entry.FolderID] = entry.AccountID
		}
		for _, folderID := range sortedMessageGroupKeys(byFolder) {
			group := byFolder[folderID]
			destination, err := o.runtime.folders.Get(folderID)
			if err != nil {
				return err
			}
			if destination == nil || destination.AccountID != folderAccounts[folderID] {
				return errors.New("original folder is unavailable for undo")
			}
			if err := o.moveToFolder(group, destination); err != nil {
				return fmt.Errorf("restore original message folder: %w", err)
			}
		}
	case v1.ActionMarkRead, v1.ActionMarkUnread, v1.ActionStar, v1.ActionUnstar:
	default:
		return fmt.Errorf("undo is unsupported for action %q", action)
	}

	for _, entry := range snapshot.Entries {
		if err := o.setFlags([]string{entry.MessageID}, "read", entry.IsRead); err != nil {
			return fmt.Errorf("restore prior read state: %w", err)
		}
		if err := o.setFlags([]string{entry.MessageID}, "starred", entry.IsStarred); err != nil {
			return fmt.Errorf("restore prior star state: %w", err)
		}
	}
	return nil
}

var _ v1.ActionUndoSnapshotter = (*mailboxActionOperations)(nil)
var _ v1.ActionUndoOperations = (*mailboxActionOperations)(nil)
