// P-007: pinned folders across accounts.
//
// The pin list is plain data kept in the persisted UI state. These tests pin
// down the rules the sidebar relies on: a pin is identified by account and
// folder together, toggling is idempotent per pair, order is pin order, and
// anything malformed in persisted data is dropped rather than rendered.

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

const { pinFolder, unpinFolder, togglePin, isPinned, normalizePins } = await load('../src/lib/sidebar/pins.ts')

const inbox = { accountId: 'acc-1', folderId: 'f-inbox', path: 'INBOX', name: 'Inbox' }
const work = { accountId: 'acc-2', folderId: 'f-work', path: 'Work', name: 'Work' }
const sameIdOtherAccount = { accountId: 'acc-2', folderId: 'f-inbox', path: 'INBOX', name: 'Inbox' }

test('pinFolder appends a new pin and keeps pin order', () => {
  const pins = pinFolder(pinFolder([], inbox), work)
  assert.deepEqual(pins.map(p => p.folderId), ['f-inbox', 'f-work'])
})

test('pinFolder is idempotent for the same account and folder', () => {
  const once = pinFolder([], inbox)
  const twice = pinFolder(once, inbox)
  assert.deepEqual(twice, once)
})

test('a folder id is only a duplicate within the same account', () => {
  const pins = pinFolder(pinFolder([], inbox), sameIdOtherAccount)
  assert.equal(pins.length, 2)
  assert.equal(isPinned(pins, 'acc-1', 'f-inbox'), true)
  assert.equal(isPinned(pins, 'acc-2', 'f-inbox'), true)
})

test('unpinFolder removes only the matching account and folder', () => {
  const pins = pinFolder(pinFolder(pinFolder([], inbox), sameIdOtherAccount), work)
  const after = unpinFolder(pins, 'acc-2', 'f-inbox')
  assert.deepEqual(after.map(p => `${p.accountId}/${p.folderId}`), ['acc-1/f-inbox', 'acc-2/f-work'])
})

test('unpinFolder on an absent pin is a no-op', () => {
  const pins = pinFolder([], inbox)
  assert.deepEqual(unpinFolder(pins, 'acc-9', 'f-missing'), pins)
})

test('togglePin pins an unpinned folder and unpins a pinned one', () => {
  const pinned = togglePin([], inbox)
  assert.equal(isPinned(pinned, inbox.accountId, inbox.folderId), true)
  const unpinned = togglePin(pinned, inbox)
  assert.equal(isPinned(unpinned, inbox.accountId, inbox.folderId), false)
})

test('pin operations do not mutate the input list', () => {
  const pins = pinFolder([], inbox)
  const snapshot = JSON.stringify(pins)
  pinFolder(pins, work)
  unpinFolder(pins, inbox.accountId, inbox.folderId)
  assert.equal(JSON.stringify(pins), snapshot)
})

test('normalizePins keeps valid entries and drops malformed ones', () => {
  const raw = [
    inbox,
    { accountId: '', folderId: 'f-x', path: 'X', name: 'X' },
    { accountId: 'acc-1', path: 'Y', name: 'Y' },
    'not-an-object',
    null,
    work,
  ]
  assert.deepEqual(normalizePins(raw).map(p => p.folderId), ['f-inbox', 'f-work'])
})

test('normalizePins removes duplicate pins and tolerates non-array input', () => {
  assert.deepEqual(normalizePins([inbox, inbox]), [inbox])
  assert.deepEqual(normalizePins(undefined), [])
  assert.deepEqual(normalizePins({ accountId: 'acc-1' }), [])
  assert.deepEqual(normalizePins(null), [])
})
