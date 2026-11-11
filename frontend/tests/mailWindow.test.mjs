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

test('back after a list-click retarget restores the previous message in the tab', () => {
  // Olli: a list click replaces the current reader tab's message; Back must
  // return to the previous one. App.navigateMessageHistory pushes the historic
  // scope through updateWorkspaceScope and restoreMessageVisit, so the state
  // chain is: open Spotify, retarget to the clicked thread, rebuild from the
  // historic scope, and the tab shows Spotify again.
  const inbox = { accountId: 'a', folderId: 'inbox', folderName: 'Inbox', folderType: 'inbox' }
  const opened = createMailWindowState({ ...inbox, threadId: 'spotify' })
  const retargeted = navigateMailWindow(opened, { ...inbox, threadId: 'other' })
  assert.equal(retargeted.thread?.threadId, 'other')
  const restored = createMailWindowState({ ...inbox, threadId: 'spotify' })
  assert.deepEqual(restored.thread, { threadId: 'spotify', folderId: 'inbox', accountId: 'a' })
})

test('navigation without a threadId still restores the remembered selection', () => {
  const inbox = { accountId: 'a', folderId: 'inbox', folderName: 'Inbox', folderType: 'inbox' }
  const open = selectMailWindowConversation(createMailWindowState(inbox), 'thread', 'inbox', 'a')
  const same = navigateMailWindow(open, inbox)
  assert.deepEqual(same.thread, { threadId: 'thread', folderId: 'inbox', accountId: 'a' })
})

test('back into a tab also brings the restored list row into view', () => {
  // Olli: Back must change focus in the message list, not only the tab. The
  // readerSelection effect moves the selection and keyboard target; it must
  // also scroll the restored row into view or the focus lands off-screen.
  return readFile(new URL('../src/lib/components/list/MessageList.svelte', import.meta.url), 'utf8').then(list => {
    const start = list.indexOf('const selection = readerSelection')
    assert.ok(start !== -1, 'readerSelection effect found')
    const effect = list.slice(start, start + 1400)
    assert.match(effect, /focusConversation\(readerRow\)/, 'the restored message becomes the list selection and keyboard target')
    assert.match(effect, /scrollIntoView/, 'the restored row is scrolled into view')
  })
})

test('the tab keeps watching config when only the thread changes', () => {
  // The MailWindowApp config effect used to compare only accountId+folderId
  // and return early, so a list click targeting the same folder never reached
  // the open reader tab. The applied-config identity must include the thread.
  return readFile(new URL('../src/MailWindowApp.svelte', import.meta.url), 'utf8').then(app => {
    const keyLine = app.split('\n').find(line => line.includes('const key =') && line.includes('scopeKey(config)'))
    assert.ok(keyLine, 'config identity key found')
    assert.match(keyLine, /config\.threadId/, 'the identity key considers the thread, not only the folder')
  })
})

test('a local selection never re-enters the config effect', () => {
  // Devin: in a standalone window (fixed config, no onScopeChange) a local
  // selection made windowState differ from config, the effect re-navigated,
  // produced a fresh state every pass and looped. The effect must compare
  // against a stored last-applied key and read windowState untracked.
  return readFile(new URL('../src/MailWindowApp.svelte', import.meta.url), 'utf8').then(app => {
    assert.match(app, /lastConfigKey/, 'the last applied config identity is stored')
    assert.match(app, /if \(key === lastConfigKey\) return/, 'an unchanged config identity returns early')
    assert.match(app, /untrack\(\(\) => navigateMailWindow\(windowState, config\)\)/, 'reading windowState inside the effect is untracked')
  })
})

test('a threadless config for the current folder does not resurrect a closed conversation', () => {
  // A local close pushes the same folder with no threadId; navigating would
  // restore the remembered selection and reopen the closed conversation.
  return readFile(new URL('../src/MailWindowApp.svelte', import.meta.url), 'utf8').then(app => {
    const effect = app.slice(app.indexOf('lastConfigKey'), app.indexOf('function select('))
    assert.match(effect, /!config\.threadId && scopeKey\(config\) === scopeKey\(windowState\.scope\)/, 'threadless same-folder config is not navigation')
  })
})

test('an external retarget keeps a reader tab on the viewer pane', () => {
  // Devin: reader tabs render no list; focusing messageList after a retarget
  // kills the reply/delete shortcuts until the viewer is clicked.
  return readFile(new URL('../src/MailWindowApp.svelte', import.meta.url), 'utf8').then(app => {
    const effect = app.slice(app.indexOf('lastConfigKey'), app.indexOf('function select('))
    assert.match(effect, /setFocusedPane\(readerTab \? 'viewer' : 'messageList'\)/, 'reader tabs keep viewer focus')
  })
})

test('a list click carries the subject into the retargeted tab', () => {
  // Devin: the tab kept the previous message's threadSubject, so its label
  // named the old message after a retarget.
  return Promise.all([
    readFile(new URL('../src/App.svelte', import.meta.url), 'utf8'),
    readFile(new URL('../src/lib/components/list/MessageList.svelte', import.meta.url), 'utf8'),
  ]).then(([app, list]) => {
    const handler = app.slice(app.indexOf('function handleConversationSelect'))
    assert.match(handler.slice(0, handler.indexOf('\n  }')), /threadSubject/, 'the retarget scope carries the subject')
    assert.match(list, /onConversationSelect\?\.\(conversation\._canonicalConversationId \|\| threadId, realFolderId, realAccountId, conversation\.subject\)/, 'the row click passes its subject')
  })
})
