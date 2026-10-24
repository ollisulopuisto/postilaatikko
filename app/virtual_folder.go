package app

import (
	"fmt"

	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

// ============================================================================
// Virtual Folder API - Exposed to frontend via Wails bindings
// ============================================================================

// GetVirtualFolders returns all virtual folders
func (a *App) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) {
	return a.virtualFolderStore.List()
}

// CreateVirtualFolder creates a new virtual folder
func (a *App) CreateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if vf == nil {
		return fmt.Errorf("virtual folder cannot be nil")
	}
	return a.virtualFolderStore.Create(vf)
}

// UpdateVirtualFolder updates an existing virtual folder
func (a *App) UpdateVirtualFolder(vf *virtualfolder.VirtualFolder) error {
	if vf == nil {
		return fmt.Errorf("virtual folder cannot be nil")
	}
	if vf.ID == "" {
		return fmt.Errorf("virtual folder ID cannot be empty")
	}
	return a.virtualFolderStore.Update(vf)
}

// DeleteVirtualFolder deletes a virtual folder
func (a *App) DeleteVirtualFolder(id string) error {
	if id == "" {
		return fmt.Errorf("virtual folder ID cannot be empty")
	}
	return a.virtualFolderStore.Delete(id)
}

// GetVirtualFolderConversations returns unread conversations for a virtual folder
func (a *App) GetVirtualFolderConversations(virtualFolderID string, offset, limit int, sortOrder string) ([]*message.Conversation, error) {
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return nil, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}

	// Ensure we don't query with an empty ID list, which could return no results but it's safe to check
	if len(vf.FolderIDs) == 0 {
		return []*message.Conversation{}, nil
	}

	// Always use the "unread" filter for virtual folders
	return a.messageStore.ListConversationsByFolderIDs(vf.FolderIDs, offset, limit, sortOrder, "unread")
}

// GetVirtualFolderCount returns the count of unread conversations for a virtual folder
func (a *App) GetVirtualFolderCount(virtualFolderID string) (int, error) {
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return 0, fmt.Errorf("failed to get virtual folder: %w", err)
	}
	if vf == nil {
		return 0, fmt.Errorf("virtual folder not found: %s", virtualFolderID)
	}

	if len(vf.FolderIDs) == 0 {
		return 0, nil
	}

	// Always use the "unread" filter for virtual folders
	return a.messageStore.CountConversationsByFolderIDs(vf.FolderIDs, "unread")
}
