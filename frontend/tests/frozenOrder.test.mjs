import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/components/list/frozenOrder.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { freezeOrder, updateFrozenOrder, frozenRows, appendFrozenRows } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const key = row => JSON.stringify([row.accountId, row.threadId])
const signature = row => JSON.stringify(row)
const clone = row => structuredClone(row)
const a = { accountId: 'one', threadId: 'same', unreadCount: 1 }
const b = { accountId: 'two', threadId: 'same', unreadCount: 1 }

test('selection/read and then another selection retain every identity and original group basis', () => {
  let state = freezeOrder([a, b], clone)
  state = updateFrozenOrder(state, [{ ...b, unreadCount: 0 }, { ...a, unreadCount: 0 }], key, signature)
  assert.deepEqual(frozenRows(state, key).map(key), [key(a), key(b)])
  assert.deepEqual(frozenRows(state, key).map(row => row.unreadCount), [0, 0], 'read indicators remain live')
  assert.deepEqual(state.baseline.map(row => row.unreadCount), [1, 1], 'group basis does not change on reading')
  assert.equal(state.changed.length, 2)
})
test('new arrivals, external removals and reordered rows wait until deliberate release', () => {
  const c = { accountId: 'one', threadId: 'new', unreadCount: 1 }
  const state = updateFrozenOrder(freezeOrder([a, b], clone), [c, b], key, signature)
  assert.deepEqual(frozenRows(state, key).map(key), [key(a), key(b)], 'passive removal retains the row until reconciliation')
  assert.equal(state.changed.length, 2)
  const released = freezeOrder(state.latest, clone)
  assert.deepEqual(frozenRows(released, key).map(key), [key(c), key(b)])
  assert.equal(released.changed.length, 0)
})
test('count is deduplicated and pagination adds only requested new identities', () => {
  let state = updateFrozenOrder(freezeOrder([a], clone), [{ ...a, unreadCount: 0 }], key, signature)
  state = updateFrozenOrder(state, [{ ...a, unreadCount: 0 }], key, signature)
  assert.equal(state.changed.length, 1)
  state = appendFrozenRows(state, [a, b], key, clone)
  assert.deepEqual(frozenRows(state, key).map(key), [key(a), key(b)])
  assert.equal(state.changed.length, 1)
})
