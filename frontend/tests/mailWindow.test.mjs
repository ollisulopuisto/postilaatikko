import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/windows/mailWindow.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { createMailWindowBackend, createMailWindowState, navigateMailWindow, selectMailWindowConversation } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

test('child UI-state and list-sort changes never write the primary window', async () => {
  const calls = []
  const backend = { Call: async (operation, args) => { calls.push({ operation, args }); return 'result' } }
  const initial = { selectedFolderId: 'main-inbox', sidebarWidth: 240, expandedAccounts: { a: true } }
  const first = createMailWindowBackend(backend, initial)
  const second = createMailWindowBackend(backend, initial)
  initial.expandedAccounts.a = false
  await first.SaveUIState({ selectedFolderId: 'child-sent', expandedAccounts: { a: false } })
  await first.SetMessageListSortOrder('oldest')
  assert.equal((await second.GetUIState()).selectedFolderId, 'main-inbox')
  const state = await second.GetUIState()
  state.expandedAccounts.a = false
  assert.equal((await second.GetUIState()).expandedAccounts.a, true)
  assert.equal(await first.GetMessageListSortOrder(), 'oldest')
  assert.deepEqual(calls, [])
  assert.equal(await first.GetConversation('thread', 'folder'), 'result')
  assert.deepEqual(calls, [{ operation: 'GetConversation', args: ['thread', 'folder'] }])
})

test('window navigation, account-qualified selection and widths are independent', () => {
  const inbox = { accountId: 'a', folderId: 'inbox', folderName: 'Inbox', folderType: 'inbox' }
  const sent = { ...inbox, folderId: 'sent', folderName: 'Sent', folderType: 'sent' }
  const first = selectMailWindowConversation(createMailWindowState(inbox), 'thread', 'inbox', 'a')
  const second = createMailWindowState(inbox)
  const navigated = navigateMailWindow(first, sent)
  assert.equal(navigated.thread, null)
  assert.equal(first.scope.folderId, 'inbox')
  assert.equal(second.thread, null)
  const restored = navigateMailWindow(navigated, inbox)
  assert.deepEqual(restored.thread, { threadId: 'thread', folderId: 'inbox', accountId: 'a' })
  assert.equal(selectMailWindowConversation(second, 'thread', 'inbox', 'b').thread.accountId, 'b')
})

test('navigation applies an explicit thread from the incoming scope', () => {
  // Olli: with a message open in a reader tab, clicking another message in
  // the primary list must open it in the tab. App.svelte retargets the tab by
  // pushing a scope whose account/folder match but whose threadId differs, so
  // an explicit threadId has to beat the remembered folder selection.
  const inbox = { accountId: 'a', folderId: 'inbox', folderName: 'Inbox', folderType: 'inbox' }
  const open = selectMailWindowConversation(createMailWindowState(inbox), 'spotify-thread', 'inbox', 'a')
  const retargeted = navigateMailWindow(open, { ...inbox, threadId: 'other-thread' })
  assert.deepEqual(retargeted.thread, { threadId: 'other-thread', folderId: 'inbox', accountId: 'a' })
  // The explicit thread becomes the folder's remembered selection
  const away = navigateMailWindow(retargeted, { ...inbox, folderId: 'sent', folderName: 'Sent', folderType: 'sent' })
  const back = navigateMailWindow(away, inbox)
  assert.equal(back.thread?.threadId, 'other-thread')
})

test('navigation without a threadId still restores the remembered selection', () => {
  const inbox = { accountId: 'a', folderId: 'inbox', folderName: 'Inbox', folderType: 'inbox' }
  const open = selectMailWindowConversation(createMailWindowState(inbox), 'thread', 'inbox', 'a')
  const same = navigateMailWindow(open, inbox)
  assert.deepEqual(same.thread, { threadId: 'thread', folderId: 'inbox', accountId: 'a' })
})

test('the tab keeps watching config when only the thread changes', () => {
  // The MailWindowApp config effect used to compare only accountId+folderId
  // and return early, so a list click targeting the same folder never reached
  // the open reader tab. It must compare the thread too.
  return readFile(new URL('../src/MailWindowApp.svelte', import.meta.url), 'utf8').then(app => {
    const guard = app.split('\n').find(line => line.includes('scopeKey(config)') && line.includes('return'))
    assert.ok(guard, 'config effect guard found')
    assert.match(guard, /threadId/, 'the early-return gate considers the thread, not only the folder')
  })
})
