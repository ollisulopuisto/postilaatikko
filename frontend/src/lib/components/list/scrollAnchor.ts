/** A stable row identity, including the account for cross-account views. */
export function mailRowKey(row: { threadId: string; accountId?: string }, accountId?: string | null): string {
  return JSON.stringify([row.accountId || accountId || '', row.threadId])
}

export interface VisibleRow { key: string; top: number; bottom: number }
export interface ScrollAnchor { key: string; top: number; scrollTop: number }

/** Anchor the first intersecting row, not necessarily the selection (the user may have scrolled away). */
export function captureScrollAnchor(rows: VisibleRow[], viewportTop: number, scrollTop: number): ScrollAnchor | null {
  const row = rows.find(row => row.bottom > viewportTop)
  return row ? { key: row.key, top: row.top - viewportTop, scrollTop } : null
}

/** Preserve its pixel offset, but never undo a user scroll while the DOM was updating. */
export function anchoredScrollTop(anchor: ScrollAnchor, rowTop: number, viewportTop: number, currentScrollTop: number): number | null {
  if (Math.abs(currentScrollTop - anchor.scrollTop) > 1) return null
  return Math.max(0, currentScrollTop + rowTop - viewportTop - anchor.top)
}

export function newRowsBeforeAnchor(previous: string[], next: string[], anchorKey: string): string[] {
  const index = next.indexOf(anchorKey)
  if (index < 0) return []
  const known = new Set(previous)
  return next.slice(0, index).filter(key => !known.has(key))
}

/** Fetch a bounded header-only window far enough to retain a visible row after a burst.
 * Missing/deleted/filter-excluded anchors stop after five extra pages, never scan the whole mailbox.
 */
export async function extendAnchorWindow<T>(initial: T[], anchorKey: string | null, key: (row: T) => string, nextPage: () => Promise<T[]>, maxPages = 5): Promise<T[]> {
  if (!anchorKey || initial.some(row => key(row) === anchorKey)) return initial
  const rows = new Map(initial.map(row => [key(row), row]))
  for (let page = 0; page < maxPages; page++) {
    const added = await nextPage()
    if (!added.length) break
    const size = rows.size
    for (const row of added) rows.set(key(row), row)
    if (rows.has(anchorKey) || rows.size === size) break
  }
  return [...rows.values()]
}
