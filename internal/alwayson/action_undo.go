package alwayson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
)

type mailboxActionUndoSnapshot struct {
	Action  v1.ActionKind                    `json:"action"`
	Entries []mailboxActionUndoSnapshotEntry `json:"entries"`
}

type mailboxActionUndoSnapshotEntry struct {
	AccountID         string `json:"accountId"`
	MessageID         string `json:"messageId"`
	FolderID          string `json:"folderId"`
	ExpectedAccountID string `json:"expectedAccountId"`
	ExpectedFolderID  string `json:"expectedFolderId"`
	IsRead            bool   `json:"isRead"`
	IsStarred         bool   `json:"isStarred"`
	ExpectedIsRead    bool   `json:"expectedIsRead"`
	ExpectedIsStarred bool   `json:"expectedIsStarred"`
}

func (o *mailboxActionOperations) CaptureActionUndo(ctx context.Context, req v1.ActionRequest, ids []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return nil, err
	}
	snapshot := mailboxActionUndoSnapshot{Action: req.Action, Entries: make([]mailboxActionUndoSnapshotEntry, 0, len(messages))}
	for _, msg := range messages {
		expectedAccountID, expectedFolderID, err := o.actionExpectedLocation(req, msg)
		if err != nil {
			return nil, err
		}
		expectedRead, expectedStarred := msg.IsRead, msg.IsStarred
		switch req.Action {
		case v1.ActionMarkRead:
			expectedRead = true
		case v1.ActionMarkUnread:
			expectedRead = false
		case v1.ActionStar:
			expectedStarred = true
		case v1.ActionUnstar:
			expectedStarred = false
		}
		snapshot.Entries = append(snapshot.Entries, mailboxActionUndoSnapshotEntry{
			AccountID:         msg.AccountID,
			MessageID:         msg.ID,
			FolderID:          msg.FolderID,
			ExpectedAccountID: expectedAccountID,
			ExpectedFolderID:  expectedFolderID,
			IsRead:            msg.IsRead,
			IsStarred:         msg.IsStarred,
			ExpectedIsRead:    expectedRead,
			ExpectedIsStarred: expectedStarred,
		})
	}
	return json.Marshal(snapshot)
}

func (o *mailboxActionOperations) actionExpectedLocation(req v1.ActionRequest, msg *message.Message) (string, string, error) {
	if msg == nil {
		return "", "", errors.New("undo target is unavailable")
	}
	switch req.Action {
	case v1.ActionArchive:
		target := folder.TypeArchive
		_, gmailAccount, err := o.gmailClient(msg.AccountID)
		if err != nil {
			return "", "", err
		}
		if gmailAccount {
			target = folder.TypeAll
		}
		destination, err := o.runtime.folders.GetByType(msg.AccountID, target)
		if err != nil {
			return "", "", err
		}
		if destination == nil && target == folder.TypeAll {
			destination, err = o.runtime.folders.GetByPath(msg.AccountID, "[Gmail]/All Mail")
			if err != nil {
				return "", "", err
			}
		}
		if destination == nil {
			return "", "", errors.New("archive destination is unavailable for undo snapshot")
		}
		return destination.AccountID, destination.ID, nil
	case v1.ActionTrash:
		destination, err := o.runtime.folders.GetByType(msg.AccountID, folder.TypeTrash)
		if err != nil {
			return "", "", err
		}
		if destination == nil {
			return "", "", errors.New("Trash folder is unavailable for undo snapshot")
		}
		return destination.AccountID, destination.ID, nil
	case v1.ActionMoveToInbox:
		destination, err := o.runtime.folders.GetByType(msg.AccountID, folder.TypeInbox)
		if err != nil {
			return "", "", err
		}
		if destination == nil {
			return "", "", errors.New("Inbox folder is unavailable for undo snapshot")
		}
		return destination.AccountID, destination.ID, nil
	case v1.ActionMoveToSpam:
		destination, err := o.runtime.folders.GetByType(msg.AccountID, folder.TypeSpam)
		if err != nil {
			return "", "", err
		}
		if destination == nil {
			return "", "", errors.New("Spam folder is unavailable for undo snapshot")
		}
		return destination.AccountID, destination.ID, nil
	case v1.ActionMove:
		destination, err := o.runtime.folders.Get(req.Destination)
		if err != nil {
			return "", "", err
		}
		if destination == nil || destination.AccountID == "" || destination.Path == "" {
			return "", "", errors.New("move destination is unavailable for undo snapshot")
		}
		return destination.AccountID, destination.ID, nil
	default:
		return msg.AccountID, msg.FolderID, nil
	}
}

func (o *mailboxActionOperations) UndoAction(ctx context.Context, action v1.ActionKind, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch action {
	case v1.ActionReplyLaterAdd, v1.ActionReplyLaterRemove, v1.ActionReadLaterAdd, v1.ActionReadLaterRemove, v1.ActionSnooze, v1.ActionUnsnooze:
		return o.undoTriageAction(ctx, action, payload)
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
		if entry.AccountID == "" || entry.MessageID == "" || entry.FolderID == "" || entry.ExpectedAccountID == "" || entry.ExpectedFolderID == "" {
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
		if entry.ExpectedAccountID != msg.AccountID || entry.ExpectedFolderID != msg.FolderID {
			return errors.New("action undo target location changed since the operation")
		}
		if entry.ExpectedIsRead != msg.IsRead || entry.ExpectedIsStarred != msg.IsStarred {
			return errors.New("action undo target flags changed since the operation")
		}
	}

	switch action {
	case v1.ActionArchive, v1.ActionTrash, v1.ActionMove, v1.ActionMoveToInbox, v1.ActionMoveToSpam:
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

func (o *mailboxActionOperations) ActionUndoAffectedRefs(ctx context.Context, payload []byte) ([]v1.AffectedMessageRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var snapshot mailboxActionUndoSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, fmt.Errorf("decode action undo snapshot: %w", err)
	}
	switch snapshot.Action {
	case v1.ActionReplyLaterAdd, v1.ActionReplyLaterRemove, v1.ActionReadLaterAdd, v1.ActionReadLaterRemove, v1.ActionSnooze, v1.ActionUnsnooze:
		return []v1.AffectedMessageRef{}, nil
	}
	ids := make([]string, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		ids = append(ids, entry.MessageID)
	}
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return nil, err
	}
	refs := make([]v1.AffectedMessageRef, 0, len(messages))
	for _, msg := range messages {
		refs = append(refs, v1.AffectedMessageRef{AccountID: msg.AccountID, MessageID: msg.ID, FolderID: msg.FolderID})
	}
	return refs, nil
}

func (o *mailboxActionOperations) CombineActionUndoSnapshots(ctx context.Context, payloads [][]byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return combineActionUndoSnapshots(payloads)
}

func combineActionUndoSnapshots(payloads [][]byte) ([]byte, error) {
	combined := mailboxActionUndoSnapshot{Entries: []mailboxActionUndoSnapshotEntry{}}
	seen := make(map[string]struct{})
	for _, payload := range payloads {
		if len(payload) == 0 {
			continue
		}
		var snapshot mailboxActionUndoSnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return nil, fmt.Errorf("decode action group undo snapshot: %w", err)
		}
		if snapshot.Action == "" || (combined.Action != "" && combined.Action != snapshot.Action) {
			return nil, errors.New("action group undo snapshots do not match")
		}
		combined.Action = snapshot.Action
		for _, entry := range snapshot.Entries {
			if entry.MessageID == "" {
				return nil, errors.New("action group undo snapshot has incomplete identity")
			}
			if _, exists := seen[entry.MessageID]; exists {
				return nil, errors.New("action group undo snapshots contain duplicate messages")
			}
			seen[entry.MessageID] = struct{}{}
			combined.Entries = append(combined.Entries, entry)
		}
	}
	if len(combined.Entries) == 0 {
		return nil, nil
	}
	return json.Marshal(combined)
}

var _ v1.ActionUndoSnapshotter = (*mailboxActionOperations)(nil)
var _ v1.ActionUndoOperations = (*mailboxActionOperations)(nil)
var _ v1.ActionGroupUndoSnapshotCombiner = (*mailboxActionOperations)(nil)
