// Archive-and-mark-read remembers which messages were unread and marks them
// unread again when that operation is undone. The snapshot store is bounded.
import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { createServer } from 'vite'

const frontendDir = fileURLToPath(new URL('..', import.meta.url))
const server = await createServer({
  root: frontendDir,
  configFile: path.join(frontendDir, 'vite.config.ts'),
  logLevel: 'error',
  server: { middlewareMode: true, hmr: false, watch: null },
  appType: 'custom',
  optimizeDeps: { noDiscovery: true, include: [] },
})
after(() => server.close())
const m = await server.ssrLoadModule('/src/lib/components/list/undoMutationEvents.ts')
const counts = await server.ssrLoadModule('/src/lib/components/list/readCounts.ts')

globalThis.window = { dispatchEvent() {} }
globalThis.CustomEvent = class { constructor(type, init) { this.type = type; this.detail = init?.detail } }

test('undo marks the remembered unread messages unread again, once', () => {
  const calls = []
  m.setUnreadRestorer(async ids => { calls.push(ids) })
  m.rememberUnreadForUndo('op-1', ['a', 'b'])
  m.publishUndoOperationCompleted('op-1')
  m.publishUndoOperationCompleted('op-1')
  assert.deepEqual(calls, [['a', 'b']])
})

test('an operation with nothing unread stores nothing', () => {
  const before = m.unreadSnapshotCount()
  m.rememberUnreadForUndo('op-none', [])
  assert.equal(m.unreadSnapshotCount(), before)
})

test('an old operation is still undoable after more than 100 newer ones', () => {
  const calls = []
  m.setUnreadRestorer(async ids => { calls.push(ids) })
  m.rememberUnreadForUndo('old-op', ['old'])
  for (let i = 0; i < 150; i++) m.rememberUnreadForUndo(`many-${i}`, [`m${i}`])
  m.publishUndoOperationCompleted('old-op')
  assert.deepEqual(calls, [['old']])
})

test('snapshots older than the retention window are dropped, newer ones kept', () => {
  const calls = []
  m.setUnreadRestorer(async ids => { calls.push(ids) })
  const t0 = 1_000_000
  m.rememberUnreadForUndo('ttl-old', ['x'], t0)
  m.rememberUnreadForUndo('ttl-new', ['y'], t0 + m.UNREAD_SNAPSHOT_TTL_MS + 1)
  m.publishUndoOperationCompleted('ttl-old')
  assert.deepEqual(calls, [])
  m.publishUndoOperationCompleted('ttl-new')
  assert.deepEqual(calls, [['y']])
})

test('read changes update search rows too, so a search row read earlier is not snapshotted unread', () => {
  const list = [{ messageIds: ['a'], unreadCount: 1 }]
  const search = [{ messageIds: ['a'], unreadCount: 1 }, { messageIds: ['b'], unreadCount: 1 }]
  const seen = new Set()
  counts.applyReadDelta(list, ['a'], true, seen)
  assert.equal(counts.applyReadDelta(search, ['a'], true, seen), true)
  assert.deepEqual(search.map(r => r.unreadCount), [0, 1])
})

test('a row shared by two lists is counted once', () => {
  const row = { messageIds: ['a', 'b'], unreadCount: 2 }
  const seen = new Set()
  counts.applyReadDelta([row], ['a'], true, seen)
  counts.applyReadDelta([row], ['a'], true, seen)
  assert.equal(row.unreadCount, 1)
})

test('expired snapshots are pruned without another archive', () => {
  const calls = []
  m.setUnreadRestorer(async ids => { calls.push(ids) })
  const t0 = 5_000_000
  m.rememberUnreadForUndo('exp-1', ['a'], t0)
  m.rememberUnreadForUndo('exp-2', ['b'], t0 + 1000)
  const before = m.unreadSnapshotCount(t0 + 2000)
  // Only time passes: no new archive happens.
  m.pruneExpiredUnreadSnapshots(t0 + m.UNREAD_SNAPSHOT_TTL_MS + 500)
  assert.equal(m.unreadSnapshotCount(t0 + m.UNREAD_SNAPSHOT_TTL_MS + 500), before - 1, 'exp-1 is dropped, exp-2 is not yet')
  m.pruneExpiredUnreadSnapshots(t0 + m.UNREAD_SNAPSHOT_TTL_MS + 2000)
  assert.equal(m.unreadSnapshotCount(t0 + m.UNREAD_SNAPSHOT_TTL_MS + 2000), before - 2, 'exp-2 is dropped too')
})
