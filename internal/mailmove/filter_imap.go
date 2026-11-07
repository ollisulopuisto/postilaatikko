package mailmove

import (
	"context"
	"fmt"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/message"
)

type filterMoveFolders interface {
	Get(id string) (*folder.Folder, error)
	GetByType(accountID string, folderType folder.Type) (*folder.Folder, error)
}

type filterMoveMessages interface {
	FilterMessageReader
	MoveMessages(ids []string, destinationFolderID string) error
	DeleteBatch(ids []string) error
}

type rawMessageFetcher interface {
	FetchRawMessage(ctx context.Context, accountID, folderID string, uid uint32) ([]byte, error)
}

// IMAPFilterMover runs the helper's provider operation for a matched filter.
// It copies to the destination first, then moves the source into its Trash
// folder. The app adapter continues to use its undo-aware move path.
type IMAPFilterMover struct {
	Messages filterMoveMessages
	Folders  filterMoveFolders
	Raw      rawMessageFetcher
	Pool     *imap.Pool
}

// MoveFilterMessage moves one cached message to a different account using
// IMAP. It leaves the source untouched unless the destination APPEND succeeds.
func (m *IMAPFilterMover) MoveFilterMessage(ctx context.Context, sourceSnapshot *message.Message, destinationFolderID string) error {
	if m == nil || m.Messages == nil || m.Folders == nil || m.Raw == nil || m.Pool == nil {
		return fmt.Errorf("IMAP filter mover is not configured")
	}
	if ctx == nil {
		return fmt.Errorf("IMAP filter move requires a context")
	}
	if sourceSnapshot == nil {
		return fmt.Errorf("IMAP filter move requires a source snapshot")
	}
	messages, err := m.Messages.GetByIDs([]string{sourceSnapshot.ID})
	if err != nil {
		return fmt.Errorf("load filter message: %w", err)
	}
	if len(messages) != 1 || messages[0] == nil {
		return fmt.Errorf("filter message is no longer available")
	}
	msg := messages[0]
	if msg.AccountID != sourceSnapshot.AccountID || msg.FolderID != sourceSnapshot.FolderID || msg.UID != sourceSnapshot.UID {
		return fmt.Errorf("filter message changed since it was matched")
	}
	if msg.UID == 0 || int32(msg.UID) < 0 {
		return fmt.Errorf("filter message has no stable source UID")
	}
	source, err := m.Folders.Get(msg.FolderID)
	if err != nil {
		return fmt.Errorf("load filter source folder: %w", err)
	}
	if source == nil {
		return fmt.Errorf("filter source folder is missing")
	}
	destination, err := m.Folders.Get(destinationFolderID)
	if err != nil {
		return fmt.Errorf("load filter destination folder: %w", err)
	}
	if destination == nil {
		return fmt.Errorf("filter destination folder is missing")
	}
	if source.AccountID != msg.AccountID || source.AccountID == destination.AccountID {
		return fmt.Errorf("filter move requires the current source account and a different destination account")
	}
	if source.Path == "" || destination.Path == "" {
		return fmt.Errorf("filter move requires configured IMAP folder paths")
	}
	trash, err := m.Folders.GetByType(source.AccountID, folder.TypeTrash)
	if err != nil {
		return fmt.Errorf("load source Trash folder: %w", err)
	}
	if trash == nil && source.Type != folder.TypeTrash {
		return fmt.Errorf("source account has no Trash folder; source preserved")
	}
	if trash != nil && trash.Path == "" {
		return fmt.Errorf("source Trash folder has no configured IMAP path; source preserved")
	}

	return CopyBeforeCleanup(func() error {
		raw, fetchErr := m.Raw.FetchRawMessage(ctx, msg.AccountID, msg.FolderID, msg.UID)
		if fetchErr != nil {
			return fmt.Errorf("fetch source message for destination copy: %w", fetchErr)
		}
		return m.withConnection(ctx, destination.AccountID, func(client *imap.Client) error {
			if _, selectErr := client.SelectMailbox(ctx, destination.Path); selectErr != nil {
				return fmt.Errorf("select destination folder: %w", selectErr)
			}
			uid, appendErr := client.AppendMessage(destination.Path, appendFlags(msg), msg.Date, raw)
			if appendErr != nil {
				return fmt.Errorf("append destination copy: %w", appendErr)
			}
			if uid == 0 {
				return fmt.Errorf("destination APPEND returned no UID")
			}
			return nil
		})
	}, func() error {
		if source.Type == folder.TypeTrash {
			if err := m.deleteSource(ctx, msg, source); err != nil {
				return err
			}
			if err := m.Messages.DeleteBatch([]string{msg.ID}); err != nil {
				return fmt.Errorf("remove trashed source from local cache: %w", err)
			}
			return nil
		}
		if err := m.moveSourceToTrash(ctx, msg, source, trash); err != nil {
			return err
		}
		if err := m.Messages.MoveMessages([]string{msg.ID}, trash.ID); err != nil {
			return fmt.Errorf("update locally cached source folder: %w", err)
		}
		return nil
	})
}

func (m *IMAPFilterMover) moveSourceToTrash(ctx context.Context, msg *message.Message, source, trash *folder.Folder) error {
	return m.withConnection(ctx, source.AccountID, func(client *imap.Client) error {
		if _, err := client.SelectMailbox(ctx, source.Path); err != nil {
			return fmt.Errorf("select filter source folder: %w", err)
		}
		if _, err := client.CopyMessages([]goimap.UID{goimap.UID(msg.UID)}, trash.Path); err != nil {
			return fmt.Errorf("copy filter source to Trash: %w", err)
		}
		if err := deleteCopiedSource(client, goimap.UID(msg.UID)); err != nil {
			return fmt.Errorf("remove filter source after Trash copy: %w", err)
		}
		return nil
	})
}

func (m *IMAPFilterMover) deleteSource(ctx context.Context, msg *message.Message, source *folder.Folder) error {
	return m.withConnection(ctx, msg.AccountID, func(client *imap.Client) error {
		if _, err := client.SelectMailbox(ctx, source.Path); err != nil {
			return fmt.Errorf("select trashed filter source: %w", err)
		}
		if err := deleteCopiedSource(client, goimap.UID(msg.UID)); err != nil {
			return fmt.Errorf("delete trashed filter source: %w", err)
		}
		return nil
	})
}

func deleteCopiedSource(client *imap.Client, uid goimap.UID) error {
	if client.Caps().Has(goimap.CapUIDPlus) {
		return client.DeleteMessageByUID(uid)
	}
	// Without UID EXPUNGE, a broad EXPUNGE can delete unrelated messages that
	// another client marked \\Deleted. Leave this copied source flagged for
	// provider cleanup instead of expunging other mailbox entries.
	return client.AddMessageFlags([]goimap.UID{uid}, []goimap.Flag{goimap.FlagDeleted})
}

func (m *IMAPFilterMover) withConnection(ctx context.Context, accountID string, operation func(*imap.Client) error) error {
	pooled, err := m.Pool.GetConnection(ctx, accountID)
	if err != nil {
		return err
	}
	err = operation(pooled.Client())
	if imap.IsConnectionError(err) {
		m.Pool.Discard(pooled)
		return err
	}
	m.Pool.Release(pooled)
	return err
}

func appendFlags(msg *message.Message) []goimap.Flag {
	flags := make([]goimap.Flag, 0, 4)
	if msg.IsRead {
		flags = append(flags, goimap.FlagSeen)
	}
	if msg.IsStarred {
		flags = append(flags, goimap.FlagFlagged)
	}
	if msg.IsAnswered {
		flags = append(flags, goimap.FlagAnswered)
	}
	if msg.IsDraft {
		flags = append(flags, goimap.FlagDraft)
	}
	return flags
}
