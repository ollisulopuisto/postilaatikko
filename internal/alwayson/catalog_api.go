package alwayson

import (
	"errors"
	"net/http"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

type catalogReadOperations struct {
	accounts       *account.Store
	folders        *folder.Store
	virtualFolders *virtualfolder.Store
}

func (o *catalogReadOperations) GetAccounts() ([]*account.Account, error) {
	return o.accounts.List()
}

func (o *catalogReadOperations) GetFolders(accountID string) ([]*folder.Folder, error) {
	return o.folders.List(accountID)
}

func (o *catalogReadOperations) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) {
	return o.virtualFolders.List()
}

// CatalogAPIHandler creates the headless account/folder/saved-view API routes.
// Mount its handler under the authenticated automation server's /v1/ prefix;
// this handler deliberately does not provide its own listener or auth layer.
func (r *SyncRuntime) CatalogAPIHandler() (http.Handler, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	operations := &catalogReadOperations{
		accounts:       account.NewStore(r.db),
		folders:        folder.NewStore(r.db),
		virtualFolders: virtualfolder.NewStore(r.db),
	}
	mux := http.NewServeMux()
	v1.MountCatalogRoutes(mux, operations, automation.RequireGrantCapability)
	return mux, nil
}
