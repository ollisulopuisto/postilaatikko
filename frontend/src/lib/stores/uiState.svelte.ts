// UI State persistence store
// Handles saving and loading UI state across app sessions

// @ts-ignore - wailsjs bindings
import { GetUIState, SaveUIState } from '../../../wailsjs/go/app/App'
// @ts-ignore - wailsjs bindings
import { appstate } from '../../../wailsjs/go/models'
import { normalizePins, type PinnedFolder } from '../sidebar/pins'
import { normalizeLayout, type SidebarLayout } from '../sidebar/layout'
import { applyNarrowerDefaults, PANE_WIDTH_DEFAULTS } from '../layout/paneWidths'
import { normalizeReaderMode, type ReaderMode } from '../components/viewer/readerMode'

export interface UIState {
  selectedAccountId: string | null
  selectedFolderId: string | null
  selectedFolderName: string
  selectedFolderType: string | null
  selectedThreadId: string | null
  selectedConversationAccountId: string | null
  selectedConversationFolderId: string | null
  sidebarWidth: number
  listWidth: number
  sidebarCollapsed: boolean
  // Sidebar section expand/collapse states
  expandedAccounts: Record<string, boolean>  // accountId -> isExpanded (default: true)
  unifiedInboxExpanded: boolean              // Unified Inbox section (default: true)
  collapsedFolders: Record<string, boolean>  // folderId -> isCollapsed (default: true/collapsed, false = explicitly expanded)
  // Folders pinned to the sidebar, in pin order (see lib/sidebar/pins.ts)
  pinnedFolders: PinnedFolder[]
  // Order and hidden blocks of the sidebar (see lib/sidebar/layout.ts)
  sidebarLayout: SidebarLayout
  // How the reader shows a thread: conversation (default) or traditional.
  readerMode: ReaderMode
  // Active extension pane: 'mail' (default) or an extension id like 'contacts'.
  activeExtension: string
}

// Pane width constraints
const SIDEBAR_MIN = 230
const SIDEBAR_MAX = 400
const LIST_MIN = 280
const LIST_MAX = 600
const SIDEBAR_LAYOUT_MIGRATION_KEY = 'eterno-mail:sidebar-layout-v6'
const PANE_WIDTH_MIGRATION_KEY = 'eterno-mail:pane-widths-v1'

// Default state
const defaultState: UIState = {
  selectedAccountId: null,
  selectedFolderId: null,
  selectedFolderName: 'Inbox',
  selectedFolderType: 'inbox',
  selectedThreadId: null,
  selectedConversationAccountId: null,
  selectedConversationFolderId: null,
  sidebarWidth: PANE_WIDTH_DEFAULTS.sidebarWidth,
  listWidth: PANE_WIDTH_DEFAULTS.listWidth,
  sidebarCollapsed: false,
  expandedAccounts: {},
  unifiedInboxExpanded: true,
  collapsedFolders: {},
  pinnedFolders: [],
  sidebarLayout: normalizeLayout(undefined, undefined),
  readerMode: 'conversation',
  activeExtension: 'mail',
}

// Current state (in-memory cache)
let currentState: UIState = { ...defaultState }
let sidebarWidthState = $state(defaultState.sidebarWidth)

// Reactive signal to notify when UI state has been loaded
// Sidebar can depend on this to re-initialize expanded states
let uiStateLoadedVersion = $state(0)

// Reactive mirror of activeExtension specifically. The rail and the main pane
// swap depend on this and live in different components, so a $state at the
// module level keeps them in sync without prop drilling. currentState still
// holds the persisted value; this mirror is what consumers read.
let activeExtensionState = $state<string>('mail')

// Reactive mirror of the reader mode, read by the viewer and its toggle.
let readerModeState = $state<ReaderMode>('conversation')

// Clamp a value within bounds
function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

// Load state from backend on startup
export async function loadUIState(): Promise<UIState> {
  try {
    const state = await GetUIState()
    if (state) {
      // Map from backend model to frontend interface
      // Backend uses camelCase JSON tags that match our interface
      currentState = {
        selectedAccountId: state.selectedAccountId || null,
        selectedFolderId: state.selectedFolderId || null,
        selectedFolderName: state.selectedFolderName || 'Inbox',
        selectedFolderType: state.selectedFolderType || 'inbox',
        selectedThreadId: state.selectedThreadId || null,
        selectedConversationAccountId: state.selectedConversationAccountId || null,
        selectedConversationFolderId: state.selectedConversationFolderId || null,
        // Validate and clamp pane widths
        sidebarWidth: clamp(state.sidebarWidth || 320, SIDEBAR_MIN, SIDEBAR_MAX),
        // Migrate the old default (420px) to the more balanced 360px width.
        // Any user-selected size is preserved.
        listWidth: clamp(state.listWidth === 420 ? 360 : state.listWidth || 360, LIST_MIN, LIST_MAX),
        sidebarCollapsed: state.sidebarCollapsed === true,
        // Sidebar expand/collapse states
        expandedAccounts: state.expandedAccounts || {},
        unifiedInboxExpanded: state.unifiedInboxExpanded !== false, // default true
        collapsedFolders: state.collapsedFolders || {},
        pinnedFolders: normalizePins(state.pinnedFolders),
        sidebarLayout: normalizeLayout(state.sidebarOrder, state.sidebarHidden),
        readerMode: normalizeReaderMode(state.readerMode),
        activeExtension: state.activeExtension || 'mail',
      }
      readerModeState = currentState.readerMode
      sidebarWidthState = currentState.sidebarWidth

      // Apply the balanced default once: it enlarges text, icons, row height
      // and spacing together without making the navigation overly wide.
      if (typeof localStorage !== 'undefined' && localStorage.getItem(SIDEBAR_LAYOUT_MIGRATION_KEY) !== 'done') {
        currentState.sidebarCollapsed = false
        currentState.sidebarWidth = 320
        sidebarWidthState = 320
        localStorage.setItem(SIDEBAR_LAYOUT_MIGRATION_KEY, 'done')
        saveUIState({ sidebarCollapsed: false, sidebarWidth: 320 })
      }

      // P-039: move to the narrower default columns once, so the message has more room.
      const widthsMigrated = typeof localStorage !== 'undefined' && localStorage.getItem(PANE_WIDTH_MIGRATION_KEY) === 'done'
      const widths = applyNarrowerDefaults(
        { sidebarWidth: currentState.sidebarWidth, listWidth: currentState.listWidth },
        widthsMigrated,
      ).widths
      if (!widthsMigrated) {
        currentState.sidebarWidth = widths.sidebarWidth
        currentState.listWidth = widths.listWidth
        sidebarWidthState = widths.sidebarWidth
        localStorage.setItem(PANE_WIDTH_MIGRATION_KEY, 'done')
        saveUIState({ sidebarWidth: widths.sidebarWidth, listWidth: widths.listWidth })
      }
      activeExtensionState = currentState.activeExtension
    }
  } catch (err) {
    console.error('Failed to load UI state:', err)
  }
  // Increment version to trigger reactive updates in components waiting for state
  uiStateLoadedVersion++
  return currentState
}

// Get the reactive version number (components can depend on this to re-run effects when state loads)
export function getUIStateVersion(): number {
  return uiStateLoadedVersion
}

// Debounced save
let saveTimer: ReturnType<typeof setTimeout> | null = null

export function saveUIState(updates: Partial<UIState>): void {
  // Merge updates into current state
  currentState = { ...currentState, ...updates }

  // Clamp pane widths if updated
  if (updates.sidebarWidth !== undefined) {
    currentState.sidebarWidth = clamp(updates.sidebarWidth, SIDEBAR_MIN, SIDEBAR_MAX)
    sidebarWidthState = currentState.sidebarWidth
  }
  if (updates.listWidth !== undefined) {
    currentState.listWidth = clamp(updates.listWidth, LIST_MIN, LIST_MAX)
  }

  // Debounce: save at most once per second
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    try {
      // Convert to backend model format
      const backendState = new appstate.UIState({
        selectedAccountId: currentState.selectedAccountId || '',
        selectedFolderId: currentState.selectedFolderId || '',
        selectedFolderName: currentState.selectedFolderName,
        selectedFolderType: currentState.selectedFolderType || '',
        selectedThreadId: currentState.selectedThreadId || '',
        selectedConversationAccountId: currentState.selectedConversationAccountId || '',
        selectedConversationFolderId: currentState.selectedConversationFolderId || '',
        sidebarWidth: currentState.sidebarWidth,
        listWidth: currentState.listWidth,
        sidebarCollapsed: currentState.sidebarCollapsed,
        expandedAccounts: currentState.expandedAccounts,
        unifiedInboxExpanded: currentState.unifiedInboxExpanded,
        collapsedFolders: currentState.collapsedFolders,
        pinnedFolders: currentState.pinnedFolders,
        sidebarOrder: currentState.sidebarLayout.order,
        sidebarHidden: currentState.sidebarLayout.hidden,
        readerMode: currentState.readerMode,
        activeExtension: currentState.activeExtension,
      })
      await SaveUIState(backendState)
    } catch (err) {
      console.error('Failed to save UI state:', err)
    }
  }, 1000)
}

export function isSidebarCollapsed(): boolean {
  return currentState.sidebarCollapsed
}

export function setSidebarCollapsed(collapsed: boolean): void {
  saveUIState({ sidebarCollapsed: collapsed })
}

// Sidebar sizing is a layout preference, so it applies immediately and uses
// the same persisted state as manual divider resizing.
export function getSidebarWidth(): number {
  return sidebarWidthState
}

export function setSidebarWidth(width: number): void {
  saveUIState({ sidebarWidth: width })
}

// Helper to check if an account is expanded (defaults to true if not set)
export function isAccountExpanded(accountId: string): boolean {
  return currentState.expandedAccounts[accountId] !== false
}

// Helper to set account expanded state
export function setAccountExpanded(accountId: string, expanded: boolean): void {
  const newExpandedAccounts = { ...currentState.expandedAccounts, [accountId]: expanded }
  saveUIState({ expandedAccounts: newExpandedAccounts })
}

// Helper to check if unified inbox is expanded
export function isUnifiedInboxExpanded(): boolean {
  return currentState.unifiedInboxExpanded !== false
}

// Helper to set unified inbox expanded state
export function setUnifiedInboxExpanded(expanded: boolean): void {
  saveUIState({ unifiedInboxExpanded: expanded })
}

// Helper to check if a folder is collapsed (defaults to true/collapsed if not set)
export function isFolderCollapsed(folderId: string): boolean {
  return currentState.collapsedFolders[folderId] !== false
}

// Helper to set folder collapsed state
export function setFolderCollapsed(folderId: string, collapsed: boolean): void {
  const newCollapsedFolders = { ...currentState.collapsedFolders, [folderId]: collapsed }
  saveUIState({ collapsedFolders: newCollapsedFolders })
}

// Folders pinned to the sidebar. The list is replaced as a whole so the
// persisted state always matches what the sidebar shows.
export function getPinnedFolders(): PinnedFolder[] {
  return currentState.pinnedFolders
}

export function setPinnedFolders(pins: PinnedFolder[]): void {
  saveUIState({ pinnedFolders: pins })
}

// Sidebar block order and visibility. Like the pins, the layout is replaced whole.
export function getSidebarLayout(): SidebarLayout {
  return currentState.sidebarLayout
}

export function setSidebarLayout(layout: SidebarLayout): void {
  saveUIState({ sidebarLayout: layout })
}

// Get current state (synchronous)
export function getUIState(): UIState {
  return currentState
}

// Active extension helpers.
//
// Returns 'mail' by default so the existing mail UI keeps rendering when no
// extension has ever been opened. Switching to an extension only persists the
// name — it does NOT clear the mail selection (selectedFolderId, selectedThreadId),
// so toggling back to Mail restores the previous mail context exactly.
export function getActiveExtension(): string {
  return activeExtensionState
}

export function setActiveExtension(name: string): void {
  const value = name || 'mail'
  activeExtensionState = value
  saveUIState({ activeExtension: value })
}

// Get pane width constraints (for UI components)
export const paneConstraints = {
  sidebar: { min: SIDEBAR_MIN, max: SIDEBAR_MAX },
  list: { min: LIST_MIN, max: LIST_MAX },
}

export function getReaderMode(): ReaderMode {
  return readerModeState
}

export function setReaderMode(mode: ReaderMode): void {
  readerModeState = mode
  saveUIState({ readerMode: mode })
}
