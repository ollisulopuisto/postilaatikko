/** A row as it is actually reachable in the rendered message list. */
export type DisplayedConversation = {
  threadId: string
  unreadCount?: number
}

export type AutoSelectNextSnapshot = {
  originThreadId: string | null
  index: number
  preferUnread: boolean
}

export type DisplayGroup<T extends DisplayedConversation> = {
  id: string
  conversations: T[]
}

/** Flattens exactly the rows rendered by inbox cards, in display order. */
export function flattenVisibleInboxGroups<T extends DisplayedConversation, G extends DisplayGroup<T>>(
  groups: readonly G[],
  collapsedGroups: ReadonlySet<string>,
  chronological: boolean,
  visibleInGroup: (group: G) => readonly T[],
): T[] {
  return groups.flatMap(group =>
    chronological || !collapsedGroups.has(group.id) ? visibleInGroup(group) : []
  )
}

// Capture this before an action clears the viewer. In particular, leaving an
// unread conversation can asynchronously mark it read before the list reloads.
export function captureAutoSelectNext(
  conversations: readonly DisplayedConversation[],
  selectedThreadId: string | null,
  checkedThreadIds: ReadonlySet<string>,
): AutoSelectNextSnapshot {
  const index = checkedThreadIds.size > 0
    ? conversations.findIndex(conversation => checkedThreadIds.has(conversation.threadId))
    : conversations.findIndex(conversation => conversation.threadId === selectedThreadId)
  const origin = index >= 0 ? conversations[index] : undefined
  return {
    originThreadId: origin?.threadId ?? selectedThreadId,
    index,
    preferUnread: (origin?.unreadCount ?? 0) > 0,
  }
}

// The replacement row normally occupies the origin's former visual index.
// An unread origin prefers a later visible unread row; when none exists (or
// the origin was the last row), it falls back to the nearest remaining row
// instead of leaving the viewer on the empty "select a conversation" state.
export function findAutoSelectNext(
  conversations: readonly DisplayedConversation[],
  snapshot: AutoSelectNextSnapshot,
): DisplayedConversation | null {
  if (snapshot.index < 0 || conversations.length === 0) return null

  let start = Math.min(snapshot.index, conversations.length - 1)
  if (conversations[start]?.threadId === snapshot.originThreadId) start += 1

  // Nearest surviving row: what shifted into the origin's place, or the new
  // last row when the origin occupied the tail of the list.
  const fallback = conversations[start] ?? conversations[conversations.length - 1] ?? null

  if (!snapshot.preferUnread) return fallback

  for (let index = start; index < conversations.length; index += 1) {
    if ((conversations[index].unreadCount ?? 0) > 0) return conversations[index]
  }
  return fallback
}

/**
 * Should a completed action close the open conversation's viewer? Only when
 * the action targeted that conversation; archiving, deleting or moving other
 * rows must leave the open conversation alone. Callers that cannot name their
 * targets (undefined) keep the legacy always-close behavior.
 */
export function openConversationAffected(
  openThreadId: string | null,
  affectedThreadIds: readonly string[] | undefined,
): boolean {
  if (!openThreadId) return false
  if (!affectedThreadIds) return true
  return affectedThreadIds.includes(openThreadId)
}

/** Map message ids to the thread ids of the rendered rows that contain them. */
export function threadIdsForMessageIds<
  T extends { threadId: string; messageIds?: string[]; messages?: { id: string }[] },
>(rows: readonly T[], messageIds: readonly string[]): string[] {
  if (messageIds.length === 0) return []
  const wanted = new Set(messageIds)
  const threadIds: string[] = []
  for (const row of rows) {
    const ids = row.messageIds ?? row.messages?.map(message => message.id) ?? []
    if (ids.some(id => wanted.has(id))) threadIds.push(row.threadId)
  }
  return threadIds
}
