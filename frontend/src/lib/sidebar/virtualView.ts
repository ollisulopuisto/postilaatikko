/** Keep the open row visible when a read-state refresh would drop it from an unread view; otherwise trust the refreshed order. */
export function retainReadSelection<T extends { threadId: string; unreadCount: number }>(previous: T[], next: T[], selected: string | null, unreadView: boolean): T[] {
  if (!selected) return next
  const index = previous.findIndex(item => item.threadId === selected)
  const row = previous[index]

  if (!row) return next

  // A row the refresh still returns keeps the order the refresh gave it, so the
  // open message sits at its true chronological spot instead of staying pinned.
  if (next.some(item => item.threadId === selected)) return next

  if (!unreadView || row.unreadCount > 0) return next

  const result = [...next]
  result.splice(Math.min(index, result.length), 0, row)
  return result
}
