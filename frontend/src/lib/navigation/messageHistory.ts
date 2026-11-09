import type { MailScope } from '../windows/mailWindow'

export interface MessageVisit {
  scope: MailScope
  workspaceId: string
  selectionSource: 'unified' | 'account' | null
  triageStatus: 'all' | 'unread' | 'readLater' | 'replyLater'
}
export interface MessageHistory { entries: MessageVisit[]; index: number }
export const emptyMessageHistory = (): MessageHistory => ({ entries: [], index: -1 })
// A thread ID alone is not an identity across accounts or folder-backed providers.
const identity = (visit: MessageVisit) => JSON.stringify([
  visit.scope.accountId, visit.scope.folderId, visit.scope.folderType,
  visit.scope.threadAccountId || visit.scope.accountId,
  visit.scope.threadFolderId || visit.scope.folderId, visit.scope.threadId,
  visit.scope.searchQuery || '', visit.triageStatus,
])
export function visitMessage(history: MessageHistory, visit: MessageVisit): MessageHistory {
  if (!visit.scope.threadId) return history
  const copy = { ...visit, scope: { ...visit.scope } }
  if (history.index >= 0 && identity(history.entries[history.index]) === identity(visit)) {
    return { ...history, entries: history.entries.map((entry, i) => i === history.index ? copy : entry) }
  }
  const entries = [...history.entries.slice(0, history.index + 1), copy].slice(-100)
  return { entries, index: entries.length - 1 }
}
export function stepMessageHistory(history: MessageHistory, direction: -1 | 1): MessageHistory {
  const index = history.index + direction
  return index < 0 || index >= history.entries.length ? history : { ...history, index }
}
