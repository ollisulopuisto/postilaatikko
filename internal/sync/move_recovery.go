package sync

import (
	"context"
	"fmt"
	"github.com/emersion/go-imap/v2"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
)

// RecoverInterruptedMoves verifies only recorded destination UIDs. It neither
// imports historical mail nor dispatches new-message handlers. Unknown outcomes
// retain their original local row, including bodies and read-receipt decisions.
func (e *Engine) RecoverInterruptedMoves(ctx context.Context, accountID, folderID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	unlock, err := e.folderLocks.lock(ctx, folderID)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := e.folderStore.Get(folderID)
	if err != nil {
		return err
	}
	if f == nil || f.AccountID != accountID || f.Hidden {
		return fmt.Errorf("move recovery folder unavailable")
	}
	if _, err := e.GmailClient(ctx, accountID); err == nil {
		return fmt.Errorf("legacy IMAP move requires verified IMAP recovery")
	} else if err != gmail.ErrNotCapable {
		return err
	}
	conn, err := e.pool.GetConnection(ctx, accountID)
	if err != nil {
		return err
	}
	defer e.pool.Release(conn)
	var firstErr error
	for _, id := range ids {
		m, err := e.messageStore.Get(id)
		if err != nil {
			return err
		}
		if m == nil || m.FolderID != folderID || int32(m.UID) >= 0 {
			continue
		}
		// A previous pending item may have left the source selected.
		mailbox, err := conn.Client().SelectMailbox(ctx, f.Path)
		if err != nil {
			return err
		}
		uid, validity, err := e.messageStore.MoveUIDEvidence(folderID, id)
		if err == nil && validity != mailbox.UIDValidity {
			err = fmt.Errorf("move destination UID validity changed")
		}
		if err == nil {
			criteria := &imap.SearchCriteria{UID: []imap.UIDSet{imap.UIDSetNum(imap.UID(uid))}, NotFlag: []imap.Flag{imap.FlagDeleted}}
			if m.MessageID != "" {
				criteria.Header = []imap.SearchCriteriaHeaderField{{Key: "MESSAGE-ID", Value: m.MessageID}}
			}
			result, searchErr := conn.Client().RawClient().UIDSearch(criteria, nil).Wait()
			err = searchErr
			if err == nil {
				uids := result.AllUIDs()
				if len(uids) != 1 || uint32(uids[0]) != uid {
					err = fmt.Errorf("verified move copy no longer present")
				}
			}
		}
		if err == nil {
			sourceID, sourceUID, sourceValidity, done, stateErr := e.messageStore.MoveCleanupState(folderID, id)
			err = stateErr
			if err == nil && !done {
				source, sourceErr := e.folderStore.Get(sourceID)
				err = sourceErr
				if err == nil && (source == nil || source.AccountID != accountID || sourceUID == 0 || sourceValidity == 0) {
					err = fmt.Errorf("source cleanup identity unavailable")
				}
				if err == nil {
					selected, selectErr := conn.Client().SelectMailbox(ctx, source.Path)
					err = selectErr
					if err == nil && selected.UIDValidity != sourceValidity {
						err = fmt.Errorf("source cleanup generation changed")
					}
				}
				if err == nil {
					result, searchErr := conn.Client().RawClient().UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{imap.UIDSetNum(imap.UID(sourceUID))}}, nil).Wait()
					err = searchErr
					if err == nil && len(result.AllUIDs()) != 0 {
						err = fmt.Errorf("copy confirmed but source cleanup is incomplete")
					}
				}
				if err == nil {
					err = e.messageStore.ConfirmMoveCleanup(folderID, validity, map[string]uint32{id: uid})
				}
			}
		}
		if err == nil {
			// Source verification may have changed the selected mailbox. Check
			// the destination generation again before committing its UID.
			latest, selectErr := conn.Client().SelectMailbox(ctx, f.Path)
			err = selectErr
			if err == nil && latest.UIDValidity != validity {
				err = fmt.Errorf("move destination generation changed before reconcile")
			}
		}
		if err == nil {
			err = e.messageStore.ReconcileVerifiedMoveUID(folderID, id, uid, validity)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
