// Sidebar block order and visibility (P-008).
//
// The sidebar is a column of blocks. A layout is the order of those blocks
// plus the set of blocks the user has hidden. Home and the bottom controls are
// not blocks and are always shown.
//
// Persisted data can be stale, partial or hand-edited, so normalization always
// returns a complete layout. The helpers return new objects and never mutate.

export const SIDEBAR_BLOCKS = [
  'unified',
  'pinned',
  'virtual',
  'calendar',
  'primary',
  'secondary',
] as const

export type SidebarBlock = (typeof SIDEBAR_BLOCKS)[number]

export interface SidebarLayout {
  order: SidebarBlock[]
  hidden: SidebarBlock[]
}

export const DEFAULT_LAYOUT: SidebarLayout = {
  order: [...SIDEBAR_BLOCKS],
  hidden: [],
}

function isSidebarBlock(value: unknown): value is SidebarBlock {
  return typeof value === 'string' && (SIDEBAR_BLOCKS as readonly string[]).includes(value)
}

// Keeps known blocks once, in the saved order. Blocks that are missing, for
// example because a newer build added one, are appended in their default order.
// Anything that is not an array yields the default order.
function normalizeOrder(raw: unknown): SidebarBlock[] {
  const order: SidebarBlock[] = []
  if (Array.isArray(raw)) {
    for (const entry of raw) {
      if (isSidebarBlock(entry) && !order.includes(entry)) order.push(entry)
    }
  }
  for (const block of SIDEBAR_BLOCKS) {
    if (!order.includes(block)) order.push(block)
  }
  return order
}

function normalizeHidden(raw: unknown): SidebarBlock[] {
  if (!Array.isArray(raw)) return []
  const hidden: SidebarBlock[] = []
  for (const entry of raw) {
    if (isSidebarBlock(entry) && !hidden.includes(entry)) hidden.push(entry)
  }
  return hidden
}

// Builds a complete, valid layout from persisted order and hidden values.
export function normalizeLayout(order: unknown, hidden: unknown): SidebarLayout {
  return { order: normalizeOrder(order), hidden: normalizeHidden(hidden) }
}

// Moves a block one position up (-1) or down (1). A move past either end, or
// for a block that is not in the layout, returns the layout unchanged.
export function moveBlock(layout: SidebarLayout, block: SidebarBlock, delta: -1 | 1): SidebarLayout {
  const from = layout.order.indexOf(block)
  const to = from + delta
  if (from === -1 || to < 0 || to >= layout.order.length) return layout
  const order = [...layout.order]
  order[from] = order[to]
  order[to] = block
  return { order, hidden: layout.hidden }
}

// Hides or shows a block. The order is unchanged.
export function setBlockHidden(layout: SidebarLayout, block: SidebarBlock, hidden: boolean): SidebarLayout {
  const rest = layout.hidden.filter(entry => entry !== block)
  return { order: layout.order, hidden: hidden ? [...rest, block] : rest }
}

// The blocks to render, in order, without the hidden ones.
export function visibleBlocks(layout: SidebarLayout): SidebarBlock[] {
  return layout.order.filter(block => !layout.hidden.includes(block))
}

// Index of the block in the order, counting hidden blocks too. Used for the
// CSS order of rendered elements. Unknown blocks return -1.
export function blockPosition(layout: SidebarLayout, block: SidebarBlock): number {
  return layout.order.indexOf(block)
}
