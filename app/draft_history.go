package app

// Wails-facing wrappers around internal/drafthistory. See that package for
// the semantics: branching undo tree, content-hash deduped states, no
// automatic pruning; lifetime/clear policy wired separately.

import (
	"github.com/hkdb/aerion/internal/drafthistory"
)

func (a *App) draftHistoryStore() *drafthistory.Store {
	return drafthistory.NewStore(a.db.DB)
}

// DraftHistoryCommit stores one accepted composer edit and returns its node ID.
func (a *App) DraftHistoryCommit(draftKey, parentID, label, payload string) (string, error) {
	return a.draftHistoryStore().Commit(draftKey, parentID, label, payload)
}

// DraftHistoryTree returns the draft's full branch structure and head.
func (a *App) DraftHistoryTree(draftKey string) (*drafthistory.Tree, error) {
	return a.draftHistoryStore().Tree(draftKey)
}

// DraftHistoryState returns the composer state payload stored at a node.
func (a *App) DraftHistoryState(nodeID string) (string, error) {
	return a.draftHistoryStore().State(nodeID)
}

// DraftHistoryCheckout moves the head to an existing node (undo/redo/goto).
func (a *App) DraftHistoryCheckout(draftKey, nodeID string) error {
	return a.draftHistoryStore().Checkout(draftKey, nodeID)
}

// ClearDraftHistory deletes a draft's whole tree. Nothing calls this yet:
// wiring it to send/discard lifecycle awaits the settled product decision.
func (a *App) ClearDraftHistory(draftKey string) error {
	return a.draftHistoryStore().Clear(draftKey)
}
