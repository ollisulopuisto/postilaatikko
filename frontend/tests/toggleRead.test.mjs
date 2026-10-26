// P-037: M toggles the read state of the message target.
//
// The toggle follows the star rule: if any target message is unread, the
// action marks all of them read; otherwise it marks them unread. The focused
// row decides by its own state. The keyboard, the command panel and any
// contextual surface all go through the same target and action.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function load(relative) {
  const source = await readFile(new URL(relative, import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}

const { resolveListTarget } = await load('../src/lib/actions/target.ts')
const { runListAction } = await load('../src/lib/actions/listActions.ts')

function source({ checked = [], focused = [], checkedHasUnread = false, selectedUnread = false } = {}) {
  return {
    hasChecked: () => checked.length > 0,
    checkedIds: () => checked,
    focusedIds: () => focused,
    checkedHasUnstarred: () => false,
    selectedStarred: () => false,
    checkedHasUnread: () => checkedHasUnread,
    selectedUnread: () => selectedUnread,
  }
}

function recordingExecutor() {
  const calls = []
  const record = (name) => (...args) => { calls.push([name, ...args]) }
  return {
    calls,
    executor: {
      archive: record('archive'),
      spam: record('spam'),
      markRead: record('markRead'),
      markUnread: record('markUnread'),
      toggleStar: record('toggleStar'),
      trash: record('trash'),
    },
  }
}

test('checked messages with any unread target marking them read', () => {
  const target = resolveListTarget(source({ checked: ['m1', 'm2'], checkedHasUnread: true }))
  assert.equal(target.kind, 'checked')
  assert.equal(target.read, true)
})

test('checked messages that are all read target marking them unread', () => {
  const target = resolveListTarget(source({ checked: ['m1', 'm2'], checkedHasUnread: false }))
  assert.equal(target.read, false)
})

test('the focused row decides by its own read state when nothing is checked', () => {
  assert.equal(resolveListTarget(source({ focused: ['m3'], selectedUnread: true })).read, true)
  assert.equal(resolveListTarget(source({ focused: ['m3'], selectedUnread: false })).read, false)
})

test('no target has no read value to apply', () => {
  const target = resolveListTarget(source())
  assert.equal(target.kind, 'none')
  assert.deepEqual(target.messageIds, [])
})

test('mail.toggleRead runs markRead when the target is unread and markUnread otherwise', async () => {
  const unread = recordingExecutor()
  await runListAction('mail.toggleRead', { kind: 'focused', messageIds: ['m3'], star: true, read: true }, unread.executor)
  assert.deepEqual(unread.calls, [['markRead', ['m3']]])

  const read = recordingExecutor()
  await runListAction('mail.toggleRead', { kind: 'checked', messageIds: ['m1', 'm2'], star: true, read: false }, read.executor)
  assert.deepEqual(read.calls, [['markUnread', ['m1', 'm2']]])
})

test('mail.toggleRead does nothing without a target', async () => {
  const none = recordingExecutor()
  await runListAction('mail.toggleRead', { kind: 'none', messageIds: [], star: true, read: false }, none.executor)
  assert.deepEqual(none.calls, [])
})

// The single-key handler must not fire M with a modifier held, because
// Alt+M, Ctrl+M and Cmd+M belong to other commands.
test('LIST_TOGGLE_READ matches a bare M and nothing with a modifier', async () => {
  const { KEY } = await load('../src/lib/keyboard/shortcuts.ts')
  const event = (init) => ({ key: 'm', ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...init })
  const cases = [
    { name: 'bare m', init: {}, want: true },
    { name: 'Alt+m', init: { altKey: true }, want: false },
    { name: 'Ctrl+m', init: { ctrlKey: true }, want: false },
    { name: 'Cmd+m', init: { metaKey: true }, want: false },
    { name: 'Shift+m', init: { key: 'M', shiftKey: true }, want: false },
    { name: 'a different key', init: { key: 's' }, want: false },
  ]
  for (const { name, init, want } of cases) {
    assert.equal(KEY.LIST_TOGGLE_READ(event(init)), want, name)
  }
})
