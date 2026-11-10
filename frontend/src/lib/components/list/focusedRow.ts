export interface FocusableRow {
  threadId: string
  accountId?: string
  folderId?: string
}

/** Unlike threadId alone, this key identifies a row in unified and search views. */
export function focusedRowKey(row: FocusableRow, accountId = '', folderId = ''): string {
  return JSON.stringify([row.accountId || accountId, row.folderId || folderId, row.threadId])
}

/** Resolve a stored row identity. A stale key never falls back to another account. */
export function findFocusedRow<T extends FocusableRow>(rows: T[], key: string | null, threadId: string | null, accountId = '', folderId = ''): T | undefined {
  if (key) return rows.find(row => focusedRowKey(row, accountId, folderId) === key)
  const matching = rows.filter(row => row.threadId === threadId)
  return matching.length === 1 ? matching[0] : undefined
}
