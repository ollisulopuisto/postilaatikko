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
const unreadBeforeArchive = new Map<string, string[]>()
let restoreUnread: ((messageIds: string[]) => Promise<unknown>) | null = null

/** The host registers how to mark messages unread (keeps this module free of backend imports). */
export function setUnreadRestorer(fn: (messageIds: string[]) => Promise<unknown>) {
  restoreUnread = fn
}

export function rememberUnreadForUndo(operationId: string, unreadMessageIds: string[]) {
  if (!operationId || unreadMessageIds.length === 0) return
  unreadBeforeArchive.set(operationId, [...unreadMessageIds])
}

export function publishUndoOperationCompleted(operationId: string) {
  if (!operationId || typeof window === 'undefined') return
  const unread = unreadBeforeArchive.get(operationId)
  if (unread) {
    unreadBeforeArchive.delete(operationId)
    void restoreUnread?.(unread)?.catch(err => console.error('Restoring unread after undo failed:', err))
  }
  window.dispatchEvent(new CustomEvent<string>(UNDO_OPERATION_COMPLETED_EVENT, {
    detail: operationId,
  }))
}
