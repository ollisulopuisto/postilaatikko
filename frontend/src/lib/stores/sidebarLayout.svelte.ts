// Reactive view of the sidebar block order and visibility (P-008).
//
// The layout lives in the persisted UI state. This store mirrors it in a
// $state so the sidebar and the settings list stay in step without passing
// callbacks down the tree.

import { getSidebarLayout, setSidebarLayout } from './uiState.svelte'
import {
  blockPosition,
  moveBlock,
  normalizeLayout,
  setBlockHidden,
  visibleBlocks,
  type SidebarBlock,
  type SidebarLayout,
} from '../sidebar/layout'

let layout = $state<SidebarLayout>(getSidebarLayout())

// Writes the layout to the UI state and updates the local mirror.
function commit(next: SidebarLayout): void {
  layout = next
  setSidebarLayout(next)
}

export const sidebarLayout = {
  // Blocks in display order, including hidden ones (for the settings list).
  get order(): SidebarBlock[] {
    return layout.order
  },

  // Blocks in display order, without the hidden ones.
  get visible(): SidebarBlock[] {
    return visibleBlocks(layout)
  },

  isVisible(block: SidebarBlock): boolean {
    return !layout.hidden.includes(block)
  },

  // Position in the order, starting at 0. Used for the CSS order of elements.
  position(block: SidebarBlock): number {
    return blockPosition(layout, block)
  },

  move(block: SidebarBlock, delta: -1 | 1): void {
    commit(moveBlock(layout, block, delta))
  },

  setVisible(block: SidebarBlock, visible: boolean): void {
    commit(setBlockHidden(layout, block, !visible))
  },

  reset(): void {
    commit(normalizeLayout(undefined, undefined))
  },

  // Re-reads the layout after the UI state has been loaded from the backend.
  sync(): void {
    layout = getSidebarLayout()
  },
}
