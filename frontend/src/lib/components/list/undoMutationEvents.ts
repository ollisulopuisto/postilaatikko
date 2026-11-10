export const UNDO_OPERATION_CREATED_EVENT = 'eterno-mail:undo-operation-created'
export const UNDO_OPERATION_COMPLETED_EVENT = 'eterno-mail:undo-operation-completed'

export type UndoOperationCreatedDetail = {
  operationId: string
  messageIds: string[]
  action: string
}

export function publishUndoOperationCreated(operationId: string, messageIds: string[], action: string) {
  if (!operationId || typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<UndoOperationCreatedDetail>(UNDO_OPERATION_CREATED_EVENT, {
    detail: { operationId, messageIds: [...messageIds], action },
  }))
}

// Archive-and-mark-read is two backend calls but one user action. The backend
// undo only moves the messages back, so the ids that were unread before are
// remembered here and marked unread again when that operation is undone, from
// any undo path (toast, Cmd/Ctrl+Z, undo history).
const unreadBeforeArchive = new Map<string, { ids: string[]; at: number }>()
let restoreUnread: ((messageIds: string[]) => Promise<unknown>) | null = null

/** The host registers how to mark messages unread (keeps this module free of backend imports). */
export function setUnreadRestorer(fn: (messageIds: string[]) => Promise<unknown>) {
  restoreUnread = fn
}

// Snapshots live exactly as long as the backend can still undo the operation, never
// by count: a count cap would drop the unread state of an archive that is still
// undoable (the durable undo history has no count cap). The backend keeps operations
// for 24h; 7 days leaves room for it to grow without this module going stale, and
// each entry is a handful of ids.
export const UNREAD_SNAPSHOT_TTL_MS = 7 * 24 * 60 * 60 * 1000

let pruneTimer: ReturnType<typeof setTimeout> | null = null
const PRUNE_EVERY_MS = 60 * 60 * 1000

/** Drops snapshots past the retention window. Stops the timer once the map is empty. */
export function pruneExpiredUnreadSnapshots(now: number = Date.now()) {
  for (const [id, entry] of unreadBeforeArchive) {
    if (now - entry.at > UNREAD_SNAPSHOT_TTL_MS) unreadBeforeArchive.delete(id)
  }
  if (unreadBeforeArchive.size === 0 && pruneTimer !== null) {
    clearTimeout(pruneTimer)
    pruneTimer = null
  }
}

// While anything is remembered, an hourly timer prunes expired entries, so a session that
// stops archiving still lets go of old ids. It does not keep a Node process alive.
function schedulePrune() {
  if (pruneTimer !== null || unreadBeforeArchive.size === 0) return
  pruneTimer = setTimeout(() => {
    pruneTimer = null
    pruneExpiredUnreadSnapshots()
    schedulePrune()
  }, PRUNE_EVERY_MS)
  ;(pruneTimer as { unref?: () => void }).unref?.()
}

export function rememberUnreadForUndo(operationId: string, unreadMessageIds: string[], now: number = Date.now()) {
  if (!operationId || unreadMessageIds.length === 0) return
  pruneExpiredUnreadSnapshots(now)
  unreadBeforeArchive.delete(operationId)
  unreadBeforeArchive.set(operationId, { ids: [...unreadMessageIds], at: now })
  schedulePrune()
}

export function unreadSnapshotCount(now: number = Date.now()): number {
  pruneExpiredUnreadSnapshots(now)
  return unreadBeforeArchive.size
}

export function publishUndoOperationCompleted(operationId: string) {
  if (!operationId || typeof window === 'undefined') return
  const unread = unreadBeforeArchive.get(operationId)?.ids
  if (unread) {
    unreadBeforeArchive.delete(operationId)
    void restoreUnread?.(unread)?.catch(err => console.error('Restoring unread after undo failed:', err))
  }
  window.dispatchEvent(new CustomEvent<string>(UNDO_OPERATION_COMPLETED_EVENT, {
    detail: operationId,
  }))
}
