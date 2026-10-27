package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/undo"
	"slices"
)

type gmailUndo struct {
	undo.BaseCommand
	reverse func() error
}

func (c *gmailUndo) Execute() error { return nil }
func (c *gmailUndo) Undo() error    { return c.reverse() }
func (a *App) gmailIDs(ctx context.Context, c *gmail.Client, msgs []*message.Message) ([]string, error) {
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		id, err := a.syncEngine.GmailProviderID(ctx, c, m)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
func (a *App) gmailMutationError(accountID string, err error) error {
	if errors.Is(err, gmail.ErrInsufficientScope) || errors.Is(err, gmail.ErrReauthRequired) {
		a.emitRuntimeEvent("oauth:reauth-required", map[string]string{"accountId": accountID, "provider": "google", "error": err.Error()})
	}
	return err
}
func (a *App) gmailLabelChange(c *gmail.Client, msgs []*message.Message, source, dest *folder.Folder, copyOnly bool) error {
	ids, err := a.gmailIDs(a.ctx, c, msgs)
	if err != nil {
		return err
	}
	add, err := a.syncEngine.GmailLabelID(a.ctx, c, dest)
	if err != nil {
		return err
	}
	remove := ""
	if !copyOnly {
		remove, err = a.syncEngine.GmailLabelID(a.ctx, c, source)
		if err != nil {
			return err
		}
	}
	adds, removes := []string{}, []string{}
	if add != "" {
		adds = append(adds, add)
	}
	if remove != "" && remove != add {
		removes = append(removes, remove)
	}
	if dest.Type == folder.TypeArchive || dest.Type == folder.TypeAll {
		if !copyOnly && !slices.Contains(removes, "INBOX") {
			removes = append(removes, "INBOX")
		}
	}
	if dest.Type == folder.TypeInbox {
		removes = append(removes, "SPAM", "TRASH")
	}
	if copyOnly && add == "" {
		return nil
	}
	return a.gmailMutationError(source.AccountID, c.Modify(a.ctx, ids, adds, removes))
}
func (a *App) moveGmailMessages(msgs []*message.Message, dest *folder.Folder, recordUndo bool, operationID string) (string, error) {
	c, err := a.syncEngine.GmailClient(a.ctx, msgs[0].AccountID)
	if err != nil {
		return operationID, a.gmailMutationError(msgs[0].AccountID, err)
	}
	if msgs[0].AccountID != dest.AccountID {
		if err = a.copyMessagesAcrossAccounts(msgs, dest); err != nil {
			return operationID, err
		}
		ids := make([]string, 0, len(msgs))
		for _, m := range msgs {
			ids = append(ids, m.ID)
		}
		_, op, err := a.trashWithUndo(ids, operationID)
		return op, err
	}
	unlock, err := a.syncEngine.LockGmailAccount(a.ctx, msgs[0].AccountID)
	if err != nil {
		return operationID, err
	}
	defer unlock()
	byFolder := map[string][]*message.Message{}
	for _, m := range msgs {
		byFolder[m.FolderID] = append(byFolder[m.FolderID], m)
	}
	for sourceID, group := range byFolder {
		if sourceID == dest.ID {
			continue
		}
		source, err := a.folderStore.Get(sourceID)
		if err != nil || source == nil {
			return operationID, fmt.Errorf("source folder missing")
		}
		if err = a.gmailLabelChange(c, group, source, dest, false); err != nil {
			return operationID, err
		}
		ids := make([]string, 0, len(group))
		for _, m := range group {
			ids = append(ids, m.ID)
		}
		if err = a.messageStore.MoveGmailMessages(ids, dest.ID); err != nil {
			return operationID, err
		}
		a.emitRuntimeEvent("messages:moved", map[string]any{"messageIds": ids, "destFolderId": dest.ID})
		if recordUndo && a.undoStack != nil {
			previous := source.ID
			captured := append([]string(nil), ids...)
			cmd := &gmailUndo{BaseCommand: undo.NewBaseCommand("Move to " + dest.Name), reverse: func() error { _, err := a.moveToFolder(captured, previous, false, ""); return err }}
			operationID = a.undoStack.PushOperation(operationID, cmd, undo.OperationMetadata{Action: "move-to-folder", MessageIDs: ids})
		}
	}
	return operationID, nil
}
