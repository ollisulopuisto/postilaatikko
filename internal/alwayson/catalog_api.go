package alwayson

import (
	"errors"
	"net/http"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

type catalogReadOperations struct {
	accounts       *account.Store
	folders        *folder.Store
	virtualFolders *virtualfolder.Store
	messages       *message.Store
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

func (o *catalogReadOperations) GetFolder(folderID string) (*folder.Folder, error) {
	return o.folders.Get(folderID)
}

func (o *catalogReadOperations) ListByFolder(folderID string, offset, limit int) ([]*message.MessageHeader, error) {
	return o.messages.ListByFolder(folderID, offset, limit)
}

// CatalogAPIHandler is kept as the compatible constructor for the mailbox read
// API handler. Mount the result under the authenticated server's /v1/ prefix.
func (r *SyncRuntime) CatalogAPIHandler() (http.Handler, error) {
	return r.MailboxReadAPIHandler()
}

// MailboxReadAPIHandler creates the headless account, folder, saved-view, and
// bounded message-header routes. Mount it under the authenticated automation
// server's /v1/ prefix; this handler has no listener or authentication layer.
func (r *SyncRuntime) MailboxReadAPIHandler() (http.Handler, error) {
	if r == nil || r.db == nil || r.mailService == nil || r.syncEngine == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	operations := &catalogReadOperations{
		accounts:       r.accounts,
		folders:        r.folders,
		virtualFolders: r.virtualFolders,
		messages:       r.messages,
	}
	readOperations := &mailboxReadOperations{runtime: r}
	mux := http.NewServeMux()
	v1.MountCatalogRoutes(mux, operations, automation.RequireGrantCapability)
	v1.MountMessageHeaderRoutes(mux, operations, automation.RequireGrantCapability)
	v1.MountSearchRoute(mux, readOperations, automation.RequireGrantCapability)
	v1.MountThreadBodyRoutes(mux, readOperations, automation.RequireGrantCapability)
	return mux, nil
}
