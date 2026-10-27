/** A selected unread row stays in place after its own automatic read transition. */
export function retainReadSelection<T extends { threadId: string; unreadCount: number }>(previous: T[], next: T[], selected: string | null, unreadView: boolean): T[] {
  if (!unreadView || !selected || next.some(item => item.threadId === selected)) return next
  const index = previous.findIndex(item => item.threadId === selected)
  const row = previous[index]
  if (!row || row.unreadCount > 0) return next
  const result = [...next]
  result.splice(Math.min(index, result.length), 0, row)
  return result
}
