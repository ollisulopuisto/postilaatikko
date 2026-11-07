import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../src/lib/windows/mailWorkspaceTabs.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { findReaderWorkspaceTab, readerTabKey } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

function scope(overrides = {}) {
  return { accountId: 'unified', folderId: 'inbox', folderName: 'Inbox', folderType: 'inbox', ...overrides }
}

test('reader tab identity uses the actual conversation, not the folder view that opened it', () => {
  const tabs = [{ id: 'tab-1', kind: 'reader', scope: scope({ threadId: 'thread-1', threadAccountId: 'account-1', threadFolderId: 'folder-1', threadSubject: 'Hello' }) }]

  const fromAnotherView = scope({ accountId: 'account-2', folderId: 'archive', threadId: 'thread-1', threadAccountId: 'account-1', threadFolderId: 'folder-1' })
  assert.equal(findReaderWorkspaceTab(tabs, fromAnotherView)?.id, 'tab-1')
})

test('reader tab identity distinguishes account, folder, and thread', () => {
  const tabs = [{ id: 'tab-1', kind: 'reader', scope: scope({ threadId: 'thread-1', threadAccountId: 'account-1', threadFolderId: 'folder-1' }) }]
  const base = { threadId: 'thread-1', threadAccountId: 'account-1', threadFolderId: 'folder-1' }

  assert.equal(findReaderWorkspaceTab(tabs, scope({ ...base, threadId: 'thread-2' })), undefined)
  assert.equal(findReaderWorkspaceTab(tabs, scope({ ...base, threadAccountId: 'account-2' })), undefined)
  assert.equal(findReaderWorkspaceTab(tabs, scope({ ...base, threadFolderId: 'folder-2' })), undefined)
  assert.equal(findReaderWorkspaceTab(tabs, scope({ accountId: 'account-1', folderId: 'folder-1' })), undefined)
})

test('reader tab key falls back to the tab scope for non-unified contexts', () => {
  const first = readerTabKey(scope({ accountId: 'account-1', folderId: 'folder-1', threadId: 'thread-1' }))
  const second = readerTabKey(scope({ accountId: 'account-1', folderId: 'folder-1', threadId: 'thread-1' }))
  assert.equal(first, second)
})
