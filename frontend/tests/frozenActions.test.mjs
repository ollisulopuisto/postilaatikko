import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
const root = new URL('../src/', import.meta.url)
const files = Object.fromEntries(await Promise.all([
  'App.svelte', 'MailWindowApp.svelte', 'lib/components/common/MessageContextMenu.svelte',
  'lib/components/common/SnoozeDialog.svelte', 'lib/components/list/MessageList.svelte',
].map(async path => [path, await readFile(new URL(path, root), 'utf8')])))
function functionBody(source, name) {
  const match = source.match(new RegExp(`(?:async )?function ${name}\\([^\\n]*\\) \\{`))
  assert.ok(match, name)
  let depth = 1, i = match.index + match[0].length
  const start = i
  for (; depth && i < source.length; i++) { if (source[i] === '{') depth++; if (source[i] === '}') depth-- }
  return source.slice(start, i - 1)
}
const js = source => ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
function expression(source, prefix) {
  const start = source.indexOf(prefix) + prefix.length
  assert.ok(start >= prefix.length)
  let depth = 1, i = start
  for (; depth && i < source.length; i++) { if (source[i] === '{') depth++; if (source[i] === '}') depth-- }
  return source.slice(start, i - 1)
}

test('detached viewer forwards false completion rather than turning Later into auto-next', () => {
  const source = files['MailWindowApp.svelte']
  const expr = expression(source.slice(source.indexOf('<ConversationViewer')), 'onActionComplete={')
  const seen = []; const run = new Function('list', `return (${js(`const action = ${expr}`).replace(/^const action = /, '').replace(/;\s*$/, '')})`)({ handleActionComplete: value => seen.push(value) })
  run(false); run(true)
  assert.deepEqual(seen, [false, true])
})

test('detached compose, full reply and inline reply each release their own list', async () => {
  const source = files['MailWindowApp.svelte']; let released = 0; const calls = []
  const compose = functionBody(source, 'compose'), reply = functionBody(source, 'reply')
  const scope = { list: { releaseFrozenOrder: () => released++ }, accountStore: { accounts: [{ account: { id: 'one' } }] }, windowState: { scope: { accountId: 'one' } }, run: fn => fn(), OpenComposerWindow: (...args) => calls.push(args), GetMessage: async () => ({ accountId: 'one' }), $_: x => x }
  const make = body => new Function('scope', `with(scope) { return async function(mode, messageId) { ${js(body)} } }`)(scope)
  await make(compose)(); await make(reply)('reply', 'm')
  const expr = expression(source.slice(source.indexOf('<ConversationViewer')), 'onReplyStart={')
  new Function('list', `return (${expr})`)(scope.list)()
  assert.equal(released, 3); assert.equal(calls.length, 2)
})

test('Later context-menu change and its toast undo reconcile without auto-next', async () => {
  const body = functionBody(files['lib/components/common/MessageContextMenu.svelte'], 'toggleLater')
  const completions = [], toasts = []
  const scope = { queueTargets: [{ accountId: 'one', conversationId: 'a' }], laterActionsEnabled: true, laterBusy: false,
    readLaterQueue: { load: async () => {}, has: () => true, setWithUndo: async () => ({ changed: true, operationId: 'undo' }) },
    undoLaterChange: async () => {}, onActionComplete: value => completions.push(value), $_: x => x,
    toasts: { success: (_, actions) => toasts.push(actions), error: error => { throw new Error(error) } },
  }
  await new Function('scope', `with(scope) { return async function(kind) { ${js(body)} } }`)(scope)('readLater')
  assert.deepEqual(completions, [false])
  toasts[0][0].onClick(); await new Promise(resolve => setImmediate(resolve))
  assert.deepEqual(completions, [false, false])
})

test('primary Later action and bulk Later undo release membership without advancing selection', async () => {
  for (const bulk of [false, true]) {
    const source = files[bulk ? 'lib/components/list/MessageList.svelte' : 'App.svelte']
    const body = functionBody(source, bulk ? 'toggleSelectedLaterQueue' : 'toggleLaterQueue')
    const completions = [], notices = []
    const complete = value => completions.push(value)
    const scope = { selectedConversationAccountId: 'one', selectedAccountId: 'one', selectedThreadId: 'a',
      selectedQueueReady: true, laterQueueMutationInFlight: false, selectedQueueRefs: [{ accountId: 'one', conversationId: 'a' }],
      readLaterQueue: { load: async () => {}, has: () => true, setWithUndo: async () => ({ changed: true, operationId: 'undo' }) },
      undoLaterChange: async () => {}, messageListRef: { handleActionComplete: complete }, handleActionComplete: complete,
      $_: x => x, addToast: value => notices.push(value.actions),
      toasts: { success: (_, actions) => notices.push(actions), error: error => { throw new Error(error) } },
    }
    await new Function('scope', `with(scope) { return async function(kind) { ${js(body)} } }`)(scope)('readLater')
    assert.deepEqual(completions, [false])
    notices[0][0].onClick(); await new Promise(resolve => setImmediate(resolve))
    assert.deepEqual(completions, [false, false])
  }
})

test('Snooze undo completes all operations before reconciling restored membership', async () => {
  const body = functionBody(files['lib/components/common/SnoozeDialog.svelte'], 'snoozeUntil')
  const events = [], notices = []
  const scope = { busy: false, open: true, targets: [{ accountId: 'one', conversationId: 'a' }, { accountId: 'one', conversationId: 'b' }],
    SnoozeConversationWithUndo: async (_, id) => ({ changed: true, operationId: id }), UndoOperation: async id => events.push(id),
    onDone: () => events.push('done'), describe: () => 'later', $_: x => x,
    toasts: { success: (_, actions) => notices.push(actions), error: error => { throw new Error(error) } },
  }
  await new Function('scope', `with(scope) { return async function(wakeAt) { ${js(body)} } }`)(scope)(new Date('2026-12-01T12:00:00Z'))
  assert.deepEqual(events, ['done'])
  notices[0][0].onClick(); await new Promise(resolve => setImmediate(resolve))
  assert.deepEqual(events, ['done', 'a', 'b', 'done'])
})
