// Pure rules for the shared folder-tree / message-list column, kept out of the
// Svelte components so they can be tested as interactions.

export type ContextPaneMode = 'folders' | 'messages'
export type LayoutMode = 'full' | 'medium' | 'narrow'
export type ResponsiveView = 'default' | 'viewer' | 'sidebar'

export interface PaneState {
  contextPaneMode: ContextPaneMode
  layoutMode: LayoutMode
  responsiveView: ResponsiveView
}

/** Width of the folded pane's strip, in px. */
export const FOLDED_PANE_WIDTH = 36

/** Closing the tree always restores the message list, and in a narrow window also hides the overlay. */
export function closeFolderTree(state: PaneState): PaneState {
  return {
    ...state,
    contextPaneMode: 'messages',
    responsiveView: state.layoutMode === 'narrow' && state.responsiveView === 'sidebar' ? 'default' : state.responsiveView,
  }
}

/** Opening the tree; a narrow window also shows the overlay. */
export function openFolderTree(state: PaneState): PaneState {
  return {
    ...state,
    contextPaneMode: 'folders',
    responsiveView: state.layoutMode === 'narrow' ? 'sidebar' : state.responsiveView,
  }
}

export function toggleFolderTree(state: PaneState): PaneState {
  return state.contextPaneMode === 'folders' ? closeFolderTree(state) : openFolderTree(state)
}

/** Widths of the two panes in the shared column; the open one takes the rest. */
export function sharedPaneWidths(listWidth: number, mode: ContextPaneMode): { folders: number; messages: number } {
  const open = Math.max(FOLDED_PANE_WIDTH, listWidth - FOLDED_PANE_WIDTH)
  return mode === 'folders'
    ? { folders: open, messages: FOLDED_PANE_WIDTH }
    : { folders: FOLDED_PANE_WIDTH, messages: open }
}

/** Divider drag: measure from the shared column's left edge, which is the same in both modes. */
export function dragListWidth(clientX: number, columnLeft: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, clientX - columnLeft))
}

export type TreeEscapeAction = 'ignore' | 'clear-query' | 'close'

/** Escape while the tree is open: leave it alone when something else owns the key
 *  (open overlay, an undo-send countdown, or the composer). */
export function treeEscapeAction(opts: { treeOpen: boolean; query: string; defaultPrevented: boolean; overlayOpen: boolean; ownedElsewhere?: boolean }): TreeEscapeAction {
  if (!opts.treeOpen || opts.defaultPrevented || opts.overlayOpen || opts.ownedElsewhere) return 'ignore'
  return opts.query ? 'clear-query' : 'close'
}
