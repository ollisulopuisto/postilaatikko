// Moves the unread count of every row that holds one of `messageIds`. `seen` stops a row that
// appears in two lists (the same object) from being counted twice. Returns whether any row changed.
export function applyReadDelta(
  rows: readonly { messageIds?: string[]; unreadCount?: number }[],
  messageIds: readonly string[],
  isRead: boolean,
  seen: Set<object> = new Set(),
): boolean {
  const changedIds = new Set(messageIds)
  let changed = false
  for (const row of rows) {
    if (seen.has(row)) continue
    seen.add(row)
    const affected = (row.messageIds || []).filter(id => changedIds.has(id)).length
    if (affected > 0) {
      changed = true
      row.unreadCount = Math.max(0, (row.unreadCount || 0) + (isRead ? -affected : affected))
    }
  }
  return changed
}
