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

test('the store is capped; the oldest snapshots go first', () => {
  const calls = []
  m.setUnreadRestorer(async ids => { calls.push(ids) })
  for (let i = 0; i < m.MAX_UNREAD_SNAPSHOTS + 25; i++) m.rememberUnreadForUndo(`cap-${i}`, [`m${i}`])
  assert.ok(m.unreadSnapshotCount() <= m.MAX_UNREAD_SNAPSHOTS)
  m.publishUndoOperationCompleted('cap-0')
  assert.deepEqual(calls, [], 'evicted operation restores nothing')
  m.publishUndoOperationCompleted(`cap-${m.MAX_UNREAD_SNAPSHOTS + 24}`)
  assert.deepEqual(calls, [[`m${m.MAX_UNREAD_SNAPSHOTS + 24}`]])
})
