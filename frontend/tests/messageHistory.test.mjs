import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/navigation/messageHistory.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { emptyMessageHistory, visitMessage, stepMessageHistory } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const visit = (threadId, accountId = 'one', folderId = 'inbox', other = {}) => ({ workspaceId: 'primary', selectionSource: 'account', triageStatus: 'all', scope: { accountId, folderId, folderName: folderId, folderType: 'inbox', threadId, threadAccountId: accountId, threadFolderId: folderId }, ...other })
test('browser navigation bounds, repeated visits and forward branching', () => {
  const empty = emptyMessageHistory()
  assert.equal(stepMessageHistory(empty, -1), empty)
  assert.equal(visitMessage(empty, visit(undefined)), empty)
  let h = visitMessage(empty, visit('A'))
  h = visitMessage(h, visit('A'))
  assert.equal(h.entries.length, 1)
  h = visitMessage(h, visit('B'))
  h = visitMessage(h, visit('C'))
  assert.equal(stepMessageHistory(h, 1), h)
  h = stepMessageHistory(h, -1)
  assert.equal(h.entries[h.index].scope.threadId, 'B')
  assert.equal(stepMessageHistory(h, 1).entries[2].scope.threadId, 'C')
  h = visitMessage(h, visit('D'))
  assert.deepEqual(h.entries.map(v => v.scope.threadId), ['A', 'B', 'D'])
  assert.equal(h.index, 2)
})
test('cross-account and folder identities, search/filter context, closed tabs and defensive copies', () => {
  let h = emptyMessageHistory()
  for (const v of [visit('same'), visit('same', 'two'), visit('same', 'two', 'archive')]) h = visitMessage(h, v)
  assert.equal(h.entries.length, 3)
  const target = visit('search', 'two', 'archive', { workspaceId: 'email-tab', triageStatus: 'unread' })
  target.scope.searchQuery = 'invoice'
  h = visitMessage(h, target)
  target.scope.searchQuery = 'changed'
  assert.equal(h.entries[3].scope.searchQuery, 'invoice')
  assert.equal(h.entries[3].workspaceId, 'email-tab')
  assert.equal(h.entries[3].triageStatus, 'unread')
  const previous = stepMessageHistory(h, -1)
  assert.equal(previous.entries[previous.index].scope.accountId, 'two')
  assert.equal(previous.entries[previous.index].scope.folderId, 'archive')
})
test('history is bounded to 100 session-only visits and does not mutate prior state', () => {
  const empty = emptyMessageHistory()
  let h = empty
  for (let i = 0; i < 120; i++) h = visitMessage(h, visit(String(i)))
  assert.equal(h.entries.length, 100)
  assert.equal(h.index, 99)
  assert.equal(h.entries[0].scope.threadId, '20')
  assert.deepEqual(empty, { entries: [], index: -1 })
  assert.doesNotMatch(source, /localStorage|UndoOperation|publishUndo|composer|SaveUIState/)
})
test('App restores scope and selection without calling mailbox or composer undo; B is outside folded panes', async () => {
  const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
  const handler = app.slice(app.indexOf('  async function navigateMessageHistory'), app.indexOf('  function workspaceLabel'))
  assert.doesNotMatch(handler, /UndoOperation|runListAction|publishUndo|composer|MarkAs|Trash|Archive/)
  assert.match(handler, /restoringMessageHistory = true/)
  assert.match(handler, /finally \{ restoringMessageHistory = false \}/)
  assert.match(handler, /workspace \? workspace.id : 'primary'/)
  // Back into a tab visit retargets that tab: the historic scope (with its
  // threadId) is pushed through updateWorkspaceScope and the tab instance is
  // rebuilt from it, so Back after a list-click retarget returns to the
  // previous message in the same tab.
  assert.match(handler, /updateWorkspaceScope\(workspace\.id, \{ \.\.\.scope \}\)/)
  assert.match(handler, /restoreMessageVisit\(scope\)/)
  assert.match(handler, /setSearchQuery\(scope.searchQuery/)
  assert.match(handler, /setQuickFilter\(visit.triageStatus/)
  // One bar: the arrows sit inline at the start of the viewer pane's tab row, not in a row of their own above the list.
  assert.match(app, /<MessageHistoryBar[^\n]*\n\s*<div class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto" role="tablist"/)
})
