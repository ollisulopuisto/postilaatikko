import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
const read = p => readFile(new URL(p, import.meta.url), 'utf8')
const base = '../src/lib/components/'
const viewer = await read(base+'viewer/ConversationViewer.svelte')
const row = await read(base+'list/ConversationRow.svelte')
const list = await read(base+'list/MessageList.svelte')
const menu = await read(base+'common/MessageContextMenu.svelte')
const button = await read(base+'common/LaterQueueButton.svelte')
test('reader toolbar, per-message header and row quick actions include both queues', () => {
  assert.equal((viewer.match(/kind="readLater"/g)||[]).length, 3)
  assert.equal((viewer.match(/kind="replyLater"/g)||[]).length, 3)
  for (const kind of ['readLater','replyLater']) assert.ok(row.includes(`kind="${kind}" variant="row"`))
  assert.match(viewer, /conversationId=\{msg.threadId \|\| threadId \|\| ''\}/)
  assert.match(viewer, /accountId=\{msg.accountId \|\| accountId \|\| ''\}/)
})
test('snoozed and multi-selected context menus keep both account-qualified queue actions', () => {
  const queueMenu = menu.slice(menu.indexOf('{#if queueTargets.length > 0}'), menu.indexOf('<!-- Move/Delete actions -->'))
  assert.ok(queueMenu.indexOf("toggleLater('readLater')") < queueMenu.indexOf("folderType === 'snoozed'"))
  assert.ok(queueMenu.indexOf("toggleLater('replyLater')") < queueMenu.indexOf("folderType === 'snoozed'"))
  assert.match(row, /laterTargets=\{useMultiSelect \? selectedQueueRefs : undefined\}/)
  assert.equal((list.match(/\{selectedQueueRefs\}/g)||[]).length, 4)
  assert.match(menu, /queueTargets.map\(ref => \(\{ \.\.\.ref \}\)\)/)
  assert.match(menu, /setWithUndo\(ref.accountId, ref.conversationId, inQueue\)/)
  assert.match(menu, /replyLaterQueue.set\(ref.accountId, ref.conversationId, inQueue\)/)
})
test('HTML-body menus receive the conversation target and include both actions', async () => {
  const body = await read(base+'viewer/EmailBody.svelte')
  const message = await read(base+'viewer/ConversationMessage.svelte')
  assert.equal((message.match(/\{conversationId\}/g)||[]).length, 2)
  for (const kind of ['readLater','replyLater']) assert.ok(body.includes(`kind="${kind}" variant="menu"`))
})
test('queue buttons share existing APIs, labels, ready/busy guards, hints and operation-specific Undo', async () => {
  for (const part of ['readLaterQueue.setWithUndo(accountId, conversationId, target)', 'replyLaterQueue.set(accountId, conversationId, target)', 'UndoOperation(result.operationId)', 'aria-pressed={inQueue}', '!queue.ready', 'withShortcut(label, shortcut)', 'ariaKeyshortcuts(shortcut)']) assert.ok(button.includes(part), part)
  for (const locale of ['en','fi']) {
    const values = JSON.parse(await read('../src/lib/i18n/locales/'+locale+'.json'))
    for (const kind of ['readLater','replyLater']) for (const key of ['add','remove','added','removed','failed']) assert.ok(values[kind][key], `${locale}.${kind}.${key}`)
  }
})

test('unresolved IMAP result queue actions are disabled and excluded from bulk targets', () => {
  assert.match(list, /_queueCanonical: false/)
  assert.match(list, /queueActionsEnabled=\{result._queueCanonical === true\}/)
  assert.match(list, /conversation._queueCanonical !== false/)
  assert.match(row, /conversationId=\{queueActionsEnabled \? conversation.threadId : ''\}/)
  assert.match(menu, /if \(!laterActionsEnabled \|\| laterBusy/)
  assert.match(list, /message\?\.accountId !== account \|\| !message.threadId/)
  assert.match(list, /threadId: message.threadId, _queueCanonical: true/)
  assert.match(list, /_queueCanonical: !!msg.threadId/)
})

test('one failed selected conversation does not skip later targets or lose successful Undo receipts', async () => {
  const ts = (await import('typescript')).default
  const start = menu.indexOf('  async function toggleLater(')
  const end = menu.indexOf('  async function confirmSnooze()', start)
  const source = menu.slice(start, end) + '\nexport { toggleLater }'
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
  const calls = [], notices = []
  const queue = { load: async () => {}, has: () => false, setWithUndo: async (account, conversation) => {
    calls.push([account,conversation]); if (conversation === 'B') throw new Error('offline')
    return {changed:true,operationId:'op-'+conversation}
  } }
  const factory = new Function('queueTargets','laterActionsEnabled','laterBusy','readLaterQueue','replyLaterQueue','toasts','$_','UndoOperation', outputText.replace(/export \{ toggleLater \};?/, '')+'; return toggleLater;')
  const handler = factory(['A','B','C'].map(conversationId=>({accountId:'one',conversationId})),true,false,queue,{}, {success:(...args)=>notices.push(args),error:(...args)=>notices.push(args)},x=>x,async()=>{})
  await handler('readLater')
  assert.deepEqual(calls.map(c=>c[1]), ['A','B','C'])
  assert.equal(notices.filter(n=>n[0]==='readLater.added').length,2)
  assert.equal(notices.filter(n=>n[0]==='readLater.failed').length,1)
  assert.ok(notices.filter(n=>n[0]==='readLater.added').every(n=>n[1][0].onClick))
})
