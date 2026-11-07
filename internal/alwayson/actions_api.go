package alwayson

import (
	"context"
	"errors"
	"fmt"
	"sort"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/mailmove"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/operations"
)

type mailboxActionOperations struct{ runtime *SyncRuntime }

func (o *mailboxActionOperations) GetByIDs(ids []string) ([]*message.Message, error) {
	return o.runtime.messages.GetByIDs(ids)
}

func (o *mailboxActionOperations) GetMessageIDsInThread(accountID, threadID string) ([]string, error) {
	return o.runtime.messages.GetMessageIDsInThread(accountID, threadID)
}

func (o *mailboxActionOperations) ArchiveMessages(ids []string) (string, error) {
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return "", err
	}
	byAccount := groupMessagesByAccount(messages)
	destinations := make(map[string]*folder.Folder, len(byAccount))
	for _, accountID := range sortedMessageGroupKeys(byAccount) {
		target := folder.TypeArchive
		_, gmailAccount, err := o.gmailClient(accountID)
		if err != nil {
			return "", err
		}
		if gmailAccount {
			target = folder.TypeAll
		}
		destination, err := o.runtime.folders.GetByType(accountID, target)
		if err != nil {
			return "", err
		}
		if destination == nil && target == folder.TypeAll {
			destination, err = o.runtime.folders.GetByPath(accountID, "[Gmail]/All Mail")
			if err != nil {
				return "", err
			}
		}
		if destination == nil {
			return "", fmt.Errorf("account %s has no archive destination", accountID)
		}
		destinations[accountID] = destination
	}
	for _, accountID := range sortedMessageGroupKeys(byAccount) {
		if err := o.moveToFolder(messagesForAccount(byAccount, accountID), destinations[accountID]); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (o *mailboxActionOperations) MarkAsRead(ids []string) error {
	return o.setFlags(ids, "read", true)
}

func (o *mailboxActionOperations) MarkAsUnread(ids []string) error {
	return o.setFlags(ids, "read", false)
}

func (o *mailboxActionOperations) StarMessages(ids []string) error {
	return o.setFlags(ids, "starred", true)
}

func (o *mailboxActionOperations) UnstarMessages(ids []string) error {
	return o.setFlags(ids, "starred", false)
}

func (o *mailboxActionOperations) TrashMessages(ids []string) error {
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return err
	}
	byAccount := groupMessagesByAccount(messages)
	destinations := make(map[string]*folder.Folder, len(byAccount))
	for _, accountID := range sortedMessageGroupKeys(byAccount) {
		destination, err := o.runtime.folders.GetByType(accountID, folder.TypeTrash)
		if err != nil {
			return err
		}
		if destination == nil {
			return fmt.Errorf("account %s has no Trash folder", accountID)
		}
		destinations[accountID] = destination
	}
	for _, accountID := range sortedMessageGroupKeys(byAccount) {
		if err := o.moveToFolder(messagesForAccount(byAccount, accountID), destinations[accountID]); err != nil {
			return err
		}
	}
	return nil
}

func (o *mailboxActionOperations) MoveMessages(ids []string, destinationFolderID string) (string, error) {
	if destinationFolderID == "" {
		return "", errors.New("destination folder ID is required")
	}
	destination, err := o.runtime.folders.Get(destinationFolderID)
	if err != nil {
		return "", err
	}
	if destination == nil || destination.AccountID == "" || destination.Path == "" {
		return "", errors.New("destination folder is unavailable")
	}
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return "", err
	}
	if err := o.moveToFolder(messages, destination); err != nil {
		return "", err
	}
	return "", nil
}

func (o *mailboxActionOperations) loadActionMessages(ids []string) ([]*message.Message, error) {
	if len(ids) == 0 {
		return nil, errors.New("at least one message is required")
	}
	messages, err := o.runtime.messages.GetByIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(messages) != len(ids) {
		return nil, errors.New("one or more target messages are no longer available")
	}
	for _, msg := range messages {
		if msg == nil || msg.ID == "" || msg.AccountID == "" || msg.FolderID == "" {
			return nil, errors.New("target message has incomplete account or folder identity")
		}
	}
	return messages, nil
}

func (o *mailboxActionOperations) setFlags(ids []string, kind string, enabled bool) error {
	messages, err := o.loadActionMessages(ids)
	if err != nil {
		return err
	}
	byAccount := groupMessagesByAccount(messages)
	for _, accountID := range sortedMessageGroupKeys(byAccount) {
		group := messagesForAccount(byAccount, accountID)
		client, gmailAccount, err := o.gmailClient(accountID)
		if err != nil {
			return err
		}
		if gmailAccount {
			providerIDs := make([]string, 0, len(group))
			for _, msg := range group {
				providerID, err := o.runtime.syncEngine.GmailProviderID(o.operationContext(), client, msg)
				if err != nil {
					return err
				}
				providerIDs = append(providerIDs, providerID)
			}
			label, add := "STARRED", enabled
			if kind == "read" {
				label, add = "UNREAD", !enabled
			}
			adds, removes := []string{}, []string{}
			if add {
				adds = append(adds, label)
			} else {
				removes = append(removes, label)
			}
			if err := client.Modify(o.operationContext(), providerIDs, adds, removes); err != nil {
				return err
			}
			continue
		}
		byFolder := groupMessagesByFolder(group)
		for _, folderID := range sortedMessageGroupKeys(byFolder) {
			if err := o.setIMAPFlags(accountID, folderID, messagesForAccount(byFolder, folderID), kind, enabled); err != nil {
				return err
			}
		}
	}
	read, starred := (*bool)(nil), (*bool)(nil)
	if kind == "read" {
		read = &enabled
	}
	if kind == "starred" {
		starred = &enabled
	}
	return o.runtime.messages.UpdateFlagsBatch(ids, read, starred)
}

func (o *mailboxActionOperations) setIMAPFlags(accountID, folderID string, messages []*message.Message, kind string, enabled bool) error {
	if len(messages) == 0 {
		return nil
	}
	f, err := o.runtime.folders.Get(folderID)
	if err != nil {
		return err
	}
	if f == nil || f.AccountID != accountID || f.Path == "" {
		return fmt.Errorf("source folder %s is unavailable in account %s", folderID, accountID)
	}
	uids := make([]goimap.UID, 0, len(messages))
	for _, msg := range messages {
		if msg.AccountID != accountID || msg.FolderID != folderID || msg.UID == 0 || int32(msg.UID) < 0 {
			return fmt.Errorf("message %s has no stable UID in its source folder", msg.ID)
		}
		uids = append(uids, goimap.UID(msg.UID))
	}
	conn, err := o.runtime.pool.GetConnection(o.operationContext(), accountID)
	if err != nil {
		return err
	}
	selected, err := conn.Client().SelectMailbox(o.operationContext(), f.Path)
	if err != nil {
		o.runtime.pool.Discard(conn)
		return err
	}
	_ = selected
	defer o.runtime.pool.Release(conn)
	flag := goimap.FlagFlagged
	if kind == "read" {
		flag = goimap.FlagSeen
	}
	if enabled {
		return conn.Client().AddMessageFlags(uids, []goimap.Flag{flag})
	}
	return conn.Client().RemoveMessageFlags(uids, []goimap.Flag{flag})
}

func (o *mailboxActionOperations) moveToFolder(messages []*message.Message, destination *folder.Folder) error {
	if len(messages) == 0 {
		return nil
	}
	byAccount := groupMessagesByAccount(messages)
	for _, accountID := range sortedMessageGroupKeys(byAccount) {
		group := messagesForAccount(byAccount, accountID)
		if accountID != destination.AccountID {
			for _, msg := range group {
				if err := o.runtime.filterer.MoveFilterMessage(o.operationContext(), msg, destination.ID); err != nil {
					return err
				}
			}
			continue
		}
		client, gmailAccount, err := o.gmailClient(accountID)
		if err != nil {
			return err
		}
		if gmailAccount {
			if err := o.moveGmailMessages(client, group, destination); err != nil {
				return err
			}
			continue
		}
		byFolder := groupMessagesByFolder(group)
		for _, folderID := range sortedMessageGroupKeys(byFolder) {
			source, err := o.runtime.folders.Get(folderID)
			if err != nil {
				return err
			}
			if source == nil || source.AccountID != accountID || source.Path == "" {
				return fmt.Errorf("source folder %s is unavailable in account %s", folderID, accountID)
			}
			if source.ID == destination.ID {
				continue
			}
			if err := o.moveIMAPMessages(messagesForAccount(byFolder, folderID), source, destination); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *mailboxActionOperations) moveGmailMessages(client *gmail.Client, messages []*message.Message, destination *folder.Folder) error {
	if len(messages) == 0 {
		return nil
	}
	accountID := messages[0].AccountID
	unlock, err := o.runtime.syncEngine.LockGmailAccount(o.operationContext(), accountID)
	if err != nil {
		return err
	}
	defer unlock()
	byFolder := groupMessagesByFolder(messages)
	for _, sourceID := range sortedMessageGroupKeys(byFolder) {
		group := messagesForAccount(byFolder, sourceID)
		if sourceID == destination.ID {
			continue
		}
		source, err := o.runtime.folders.Get(sourceID)
		if err != nil {
			return err
		}
		if source == nil || source.AccountID != accountID {
			return fmt.Errorf("source folder %s is unavailable", sourceID)
		}
		providerIDs := make([]string, 0, len(group))
		for _, msg := range group {
			providerID, err := o.runtime.syncEngine.GmailProviderID(o.operationContext(), client, msg)
			if err != nil {
				return err
			}
			providerIDs = append(providerIDs, providerID)
		}
		addLabel, err := o.runtime.syncEngine.GmailLabelID(o.operationContext(), client, destination)
		if err != nil {
			return err
		}
		removeLabel, err := o.runtime.syncEngine.GmailLabelID(o.operationContext(), client, source)
		if err != nil {
			return err
		}
		adds, removes := []string{}, []string{}
		if addLabel != "" {
			adds = append(adds, addLabel)
		}
		if removeLabel != "" && removeLabel != addLabel {
			removes = append(removes, removeLabel)
		}
		if destination.Type == folder.TypeArchive || destination.Type == folder.TypeAll {
			removes = append(removes, "INBOX")
		}
		if destination.Type == folder.TypeInbox {
			removes = append(removes, "SPAM", "TRASH")
		}
		if err := client.Modify(o.operationContext(), providerIDs, adds, removes); err != nil {
			return err
		}
		ids := make([]string, 0, len(group))
		for _, msg := range group {
			ids = append(ids, msg.ID)
		}
		if err := o.runtime.messages.MoveGmailMessages(ids, destination.ID); err != nil {
			return err
		}
	}
	return nil
}

func (o *mailboxActionOperations) moveIMAPMessages(messages []*message.Message, source, destination *folder.Folder) error {
	if len(messages) == 0 {
		return nil
	}
	ordered := append([]*message.Message(nil), messages...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].UID < ordered[j].UID })
	uids := make([]goimap.UID, 0, len(ordered))
	for _, msg := range ordered {
		if msg.UID == 0 || int32(msg.UID) < 0 {
			return fmt.Errorf("message %s has no stable UID in its source folder", msg.ID)
		}
		uids = append(uids, goimap.UID(msg.UID))
	}
	accountID := ordered[0].AccountID
	conn, err := o.runtime.pool.GetConnection(o.operationContext(), accountID)
	if err != nil {
		return err
	}
	client := conn.Client()
	caps := client.Caps()
	baseline := map[string]map[uint32]bool{}
	if !caps.Has(goimap.CapUIDPlus) {
		for _, msg := range ordered {
			if msg.MessageID == "" {
				o.runtime.pool.Release(conn)
				return fmt.Errorf("cannot safely map a destination copy for message %s without UIDPLUS or Message-ID", msg.ID)
			}
		}
	}
	{
		if _, err = client.SelectMailbox(o.operationContext(), destination.Path); err != nil {
			o.runtime.pool.Discard(conn)
			return fmt.Errorf("select destination folder: %w", err)
		}
		for _, msg := range ordered {
			if msg.MessageID == "" {
				continue
			}
			if _, found := baseline[msg.MessageID]; found {
				continue
			}
			uids, err := searchMessageUIDs(client, msg.MessageID)
			if err != nil {
				o.runtime.pool.Discard(conn)
				return fmt.Errorf("snapshot destination before move: %w", err)
			}
			baseline[msg.MessageID] = uids
		}
	}
	if _, err = client.SelectMailbox(o.operationContext(), source.Path); err != nil {
		o.runtime.pool.Discard(conn)
		return fmt.Errorf("select source folder: %w", err)
	}
	destinationUIDs, err := client.CopyMessages(uids, destination.Path)
	if err != nil {
		o.runtime.pool.Discard(conn)
		return fmt.Errorf("copy messages to destination; source preserved: %w", err)
	}
	resolved := make(map[string]uint32, len(ordered))
	if len(destinationUIDs) == len(ordered) {
		for i, uid := range destinationUIDs {
			resolved[ordered[i].ID] = uint32(uid)
		}
	}
	if len(resolved) != len(ordered) {
		if _, err = client.SelectMailbox(o.operationContext(), destination.Path); err != nil {
			o.runtime.pool.Discard(conn)
			return fmt.Errorf("destination copy succeeded but source is preserved; select destination for UID resolution: %w", err)
		}
		groups := map[string][]*message.Message{}
		for _, msg := range ordered {
			if msg.MessageID == "" {
				o.runtime.pool.Release(conn)
				return fmt.Errorf("destination copy succeeded but source is preserved: no Message-ID available to resolve the new UID for %s", msg.ID)
			}
			groups[msg.MessageID] = append(groups[msg.MessageID], msg)
		}
		for messageID, group := range groups {
			candidates, err := searchMessageUIDs(client, messageID)
			if err != nil {
				o.runtime.pool.Discard(conn)
				return fmt.Errorf("destination copy succeeded but source is preserved; resolve destination UIDs: %w", err)
			}
			for uid := range baseline[messageID] {
				delete(candidates, uid)
			}
			if len(candidates) != len(group) {
				o.runtime.pool.Release(conn)
				return fmt.Errorf("destination copy succeeded but source is preserved: expected %d new destination copies for message-id %q, found %d", len(group), messageID, len(candidates))
			}
			orderedUIDs := make([]uint32, 0, len(candidates))
			for uid := range candidates {
				orderedUIDs = append(orderedUIDs, uid)
			}
			sort.Slice(orderedUIDs, func(i, j int) bool { return orderedUIDs[i] < orderedUIDs[j] })
			sort.Slice(group, func(i, j int) bool { return group[i].UID < group[j].UID })
			for i, msg := range group {
				resolved[msg.ID] = orderedUIDs[i]
			}
		}
	}
	if _, err = client.SelectMailbox(o.operationContext(), source.Path); err != nil {
		o.runtime.pool.Discard(conn)
		return fmt.Errorf("destination copy confirmed but source is preserved; reselect source: %w", err)
	}
	ids := make([]string, 0, len(ordered))
	for _, msg := range ordered {
		ids = append(ids, msg.ID)
	}
	if err := o.runtime.messages.MoveMessages(ids, destination.ID); err != nil {
		o.runtime.pool.Release(conn)
		return fmt.Errorf("destination copy confirmed but local move failed; source preserved: %w", err)
	}
	if _, err := o.runtime.messages.ReconcileMovedMessageUIDs(destination.ID, resolved); err != nil {
		o.runtime.pool.Release(conn)
		return fmt.Errorf("destination copy confirmed but local UID reconciliation failed; source preserved: %w", err)
	}
	for _, uid := range uids {
		if err := mailmove.DeleteCopiedSource(client, uid); err != nil {
			o.runtime.pool.Discard(conn)
			return fmt.Errorf("destination copy confirmed; source cleanup failed: %w", err)
		}
	}
	o.runtime.pool.Release(conn)
	return nil
}

func searchMessageUIDs(client *imap.Client, messageID string) (map[uint32]bool, error) {
	search, err := client.RawClient().UIDSearch(&goimap.SearchCriteria{
		Header:  []goimap.SearchCriteriaHeaderField{{Key: "MESSAGE-ID", Value: messageID}},
		NotFlag: []goimap.Flag{goimap.FlagDeleted},
	}, nil).Wait()
	if err != nil {
		return nil, err
	}
	uids := make(map[uint32]bool, len(search.AllUIDs()))
	for _, uid := range search.AllUIDs() {
		uids[uint32(uid)] = true
	}
	return uids, nil
}

func (o *mailboxActionOperations) gmailClient(accountID string) (*gmail.Client, bool, error) {
	client, err := o.runtime.syncEngine.GmailClient(o.operationContext(), accountID)
	if errors.Is(err, gmail.ErrNotCapable) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return client, true, nil
}

func (o *mailboxActionOperations) operationContext() context.Context {
	if o == nil || o.runtime == nil {
		return context.Background()
	}
	o.runtime.ctxMu.RLock()
	ctx := o.runtime.ctx
	o.runtime.ctxMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func groupMessagesByAccount(messages []*message.Message) map[string][]*message.Message {
	groups := make(map[string][]*message.Message)
	for _, msg := range messages {
		groups[msg.AccountID] = append(groups[msg.AccountID], msg)
	}
	return groups
}

func groupMessagesByFolder(messages []*message.Message) map[string][]*message.Message {
	groups := make(map[string][]*message.Message)
	for _, msg := range messages {
		groups[msg.FolderID] = append(groups[msg.FolderID], msg)
	}
	return groups
}

func messagesForAccount(groups map[string][]*message.Message, key string) []*message.Message {
	return groups[key]
}

func sortedMessageGroupKeys(groups map[string][]*message.Message) []string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var _ v1.ActionOperations = (*mailboxActionOperations)(nil)
var _ operations.MessageLookup = (*mailboxActionOperations)(nil)
