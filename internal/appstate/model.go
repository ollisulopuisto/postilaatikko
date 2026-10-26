package appstate

// UIState represents the persisted UI state across sessions
type UIState struct {
	// View state (what folder/view is shown in sidebar)
	SelectedAccountID  string `json:"selectedAccountId"`  // 'unified' or real account ID
	SelectedFolderID   string `json:"selectedFolderId"`   // 'inbox' (virtual) or real folder ID
	SelectedFolderName string `json:"selectedFolderName"` // Display name
	SelectedFolderType string `json:"selectedFolderType"` // inbox, sent, drafts, etc.

	// Conversation state (what's shown in viewer)
	SelectedThreadID              string `json:"selectedThreadId"`
	SelectedConversationAccountID string `json:"selectedConversationAccountId"` // Real account ID
	SelectedConversationFolderID  string `json:"selectedConversationFolderId"`  // Real folder ID

	// Pane widths
	SidebarWidth     int  `json:"sidebarWidth"`
	ListWidth        int  `json:"listWidth"`
	SidebarCollapsed bool `json:"sidebarCollapsed"`

	// Sidebar section expand/collapse states
	ExpandedAccounts     map[string]bool `json:"expandedAccounts"`     // accountID -> isExpanded (default: true)
	UnifiedInboxExpanded bool            `json:"unifiedInboxExpanded"` // Unified Inbox section (default: true)
	CollapsedFolders     map[string]bool `json:"collapsedFolders"`     // folderID -> isCollapsed (default: false/absent)

	// Folders pinned to the sidebar for one-click access, in pin order. An
	// account and folder pair identifies a pin, so folders from different
	// accounts can be pinned side by side.
	PinnedFolders []PinnedFolder `json:"pinnedFolders,omitempty"`

	// Sidebar block order and hidden blocks. Block names are owned by the
	// frontend (lib/sidebar/layout.ts), which normalizes stale or unknown
	// entries, so the backend stores them as given. Unset means the default layout.
	SidebarOrder  []string       `json:"sidebarOrder,omitempty"`
	SidebarHidden []string       `json:"sidebarHidden,omitempty"`
	SidebarGroups []SidebarGroup `json:"sidebarGroups,omitempty"`

	// Contacts and Calendar are available from the rail only when this
	// preference is enabled. Missing JSON preserves the new default (false).
	ShowContactsCalendarRail *bool `json:"showContactsCalendarRail,omitempty"`

	// Active extension pane: "mail" (default) or an extension id like "contacts".
	// The frontend uses this to decide whether to render Mail or an extension's
	// pane after the rail is shown. Missing field -> Mail.
	ActiveExtension string `json:"activeExtension,omitempty"`
}

// SidebarGroup stores a presentation-only collection of navigation entries.
type SidebarGroup struct {
	ID        string   `json:"id"`
	TitleKey  string   `json:"titleKey"`
	Entries   []string `json:"entries"`
	Collapsed bool     `json:"collapsed"`
	Hidden    bool     `json:"hidden"`
}

// PinnedFolder is a folder the user pinned to the sidebar. Name and Path are
// kept so the pin can be shown and opened without loading the folder tree.
type PinnedFolder struct {
	AccountID string `json:"accountId"`
	FolderID  string `json:"folderId"`
	Path      string `json:"path"`
	Name      string `json:"name"`
}
