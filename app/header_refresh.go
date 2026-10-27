package app

import (
	"fmt"
	"github.com/hkdb/aerion/internal/folder"
	"time"
)

// RefreshViewHeaders is nonblocking and coalesced. Opening a folder never clears
// its cache or cancels a manual sync. Metadata events update the visible list.
func (a *App) RefreshViewHeaders(accountID, folderID string) error {
	if accountID == "virtual" {
		return nil
	} // virtual views already perform a server query
	if accountID == "unified" {
		accounts, err := a.accountStore.List()
		if err != nil {
			return err
		}
		for _, acc := range accounts {
			if !acc.Enabled {
				continue
			}
			f, err := a.folderStore.GetByType(acc.ID, folder.Type(folderID))
			if err != nil {
				return err
			}
			if f != nil {
				if err = a.RefreshViewHeaders(acc.ID, f.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	f, err := a.folderStore.Get(folderID)
	if err != nil {
		return err
	}
	if f == nil || f.AccountID != accountID {
		return fmt.Errorf("folder account mismatch")
	}
	key := accountID + ":" + folderID
	a.refreshMu.Lock()
	if a.refreshLast == nil {
		a.refreshLast = map[string]time.Time{}
		a.refreshActive = map[string]bool{}
	}
	if a.refreshActive[key] || time.Since(a.refreshLast[key]) < 30*time.Second {
		a.refreshMu.Unlock()
		return nil
	}
	a.refreshActive[key] = true
	a.refreshMu.Unlock()
	go func() {
		defer recoverPanic("app.refresh", "folder metadata")
		defer func() { a.refreshMu.Lock(); delete(a.refreshActive, key); a.refreshMu.Unlock() }()
		a.syncMu.Lock()
		_, running := a.syncContexts[key]
		a.syncMu.Unlock()
		if running {
			return
		}
		if err := a.syncFolder(accountID, folderID, false); err != nil {
			a.emitRuntimeEvent("folder:syncError", map[string]string{"accountId": accountID, "folderId": folderID, "error": err.Error()})
			return
		}
		a.refreshMu.Lock()
		a.refreshLast[key] = time.Now()
		a.refreshMu.Unlock()
	}()
	return nil
}
