export interface MailScope {
  accountId: string
  folderId: string
  folderName: string
  folderType: string
  threadId?: string
  threadAccountId?: string
  threadFolderId?: string
}
export interface MailThread { accountId: string; folderId: string; threadId: string }
export interface MailWindowState {
  scope: MailScope
  thread: MailThread | null
  selections: Record<string, MailThread>
}
const scopeKey = (scope: MailScope) => JSON.stringify([scope.accountId, scope.folderId])

export function createMailWindowState(scope: MailScope): MailWindowState {
  const thread = scope.threadId ? { threadId: scope.threadId, folderId: scope.threadFolderId || scope.folderId, accountId: scope.threadAccountId || scope.accountId } : null
  return { scope: { ...scope }, thread, selections: thread ? { [scopeKey(scope)]: thread } : {} }
}
export function navigateMailWindow(state: MailWindowState, scope: MailScope): MailWindowState {
  return { ...state, scope: { ...scope }, thread: state.selections[scopeKey(scope)] ?? null }
}
export function selectMailWindowConversation(state: MailWindowState, threadId: string, folderId: string, accountId: string): MailWindowState {
  const thread = { threadId, folderId, accountId }
  return { ...state, thread, selections: { ...state.selections, [scopeKey(state.scope)]: thread } }
}

// These preferences belong to this process. In particular, viewer SaveUIState
// calls must never overwrite the main window's current folder or pane widths.
export function createMailWindowBackend(backend: { Call(operation: string, args: unknown[]): Promise<unknown> }, initialUIState: unknown): Record<string, (...args: any[]) => Promise<any>> {
  let uiState = structuredClone(initialUIState)
  let sortOrder: string | undefined
  return new Proxy({}, {
    get: (_target, operation: string) => {
      if (operation === 'GetUIState') return async () => structuredClone(uiState)
      if (operation === 'SaveUIState') return async (state: unknown) => { uiState = structuredClone(state) }
      if (operation === 'SetMessageListSortOrder') return async (value: string) => { sortOrder = value }
      if (operation === 'GetMessageListSortOrder' && sortOrder) return async () => sortOrder
      return (...args: unknown[]) => backend.Call(operation, args)
    },
  })
}
