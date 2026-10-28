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

// ClearDraftHistory deletes a draft's whole tree. Wired to actual send and
// explicit draft discard only; cancelling a pending send preserves history.
func (a *App) ClearDraftHistory(draftKey string) error {
	return a.draftHistoryStore().Clear(draftKey)
}

// MoveDraftHistory re-keys a tree when an unsaved draft first gets a real
// draft ID, keeping pre-save states reachable. See store.Move semantics.
func (a *App) MoveDraftHistory(oldKey, newKey string) error {
	return a.draftHistoryStore().Move(oldKey, newKey)
}

func (c *ComposerApp) draftHistoryStore() *drafthistory.Store {
	return drafthistory.NewStore(c.db.DB)
}

// Detached-composer twins: the child process binds only ComposerApp, so it
// needs its own history bindings against the shared database.

func (c *ComposerApp) DraftHistoryCommit(draftKey, parentID, label, payload string) (string, error) {
	return c.draftHistoryStore().Commit(draftKey, parentID, label, payload)
}

func (c *ComposerApp) DraftHistoryTree(draftKey string) (*drafthistory.Tree, error) {
	return c.draftHistoryStore().Tree(draftKey)
}

func (c *ComposerApp) DraftHistoryState(nodeID string) (string, error) {
	return c.draftHistoryStore().State(nodeID)
}

func (c *ComposerApp) DraftHistoryCheckout(draftKey, nodeID string) error {
	return c.draftHistoryStore().Checkout(draftKey, nodeID)
}

func (c *ComposerApp) ClearDraftHistory(draftKey string) error {
	return c.draftHistoryStore().Clear(draftKey)
}

func (c *ComposerApp) MoveDraftHistory(oldKey, newKey string) error {
	return c.draftHistoryStore().Move(oldKey, newKey)
}
