import type { MailScope } from './mailWindow'

export type MailWorkspaceTab = { id: string; kind: 'reader'; scope: MailScope }

/** Identity of the conversation a reader tab is showing, independent of the folder view that opened it. */
export function readerTabKey(scope: MailScope): string {
  const threadId = scope.threadId
  if (!threadId) return ''
  return JSON.stringify([scope.threadAccountId || scope.accountId, scope.threadFolderId || scope.folderId, threadId])
}

export function findReaderWorkspaceTab<T extends Pick<MailWorkspaceTab, 'kind' | 'scope'>>(tabs: readonly T[], scope: MailScope): T | undefined {
  const key = readerTabKey(scope)
  if (!key) return undefined
  return tabs.find(tab => tab.kind === 'reader' && readerTabKey(tab.scope) === key)
}
