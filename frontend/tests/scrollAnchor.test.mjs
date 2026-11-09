import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/components/list/scrollAnchor.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { mailRowKey, captureScrollAnchor, anchoredScrollTop, newRowsBeforeAnchor, extendAnchorWindow } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

test('arrival burst larger than the viewport: preserve the anchor pixel, not the old scrollTop', () => {
  const old = [{ key: 'open', top: 122, bottom: 182 }, { key: 'older', top: 182, bottom: 242 }]
  const anchor = captureScrollAnchor(old, 100, 0)
  assert.deepEqual(anchor, { key: 'open', top: 22, scrollTop: 0 })
  // 50 new rows, each 60px high; correct even when the whole viewport was previously filled.
  const corrected = anchoredScrollTop(anchor, 3122, 100, 0)
  assert.equal(corrected, 3000)
  assert.equal(3122 - corrected - 100, 22)
  const fresh = Array.from({ length: 50 }, (_, i) => `new-${i}`)
  assert.deepEqual(newRowsBeforeAnchor(['open', 'older'], [...fresh, 'open', 'older'], 'open'), fresh)
  assert.equal(newRowsBeforeAnchor([...fresh, 'open', 'older'], [...fresh, 'open', 'older'], 'open').length, 0)
})

test('partially visible row stays partially visible across variable-height inserts', () => {
  const anchor = captureScrollAnchor([{ key: 'off', top: 0, bottom: 80 }, { key: 'visible', top: 80, bottom: 150 }], 100, 200)
  assert.equal(anchor.key, 'visible')
  assert.equal(anchoredScrollTop(anchor, 247, 100, 200), 367)
  // User scrolls during DOM update: do not snap back.
  assert.equal(anchoredScrollTop(anchor, 247, 100, 250), null)
})

test('missing anchor, removed mail, pagination and cross-account collisions do not create false new mail', () => {
  assert.equal(captureScrollAnchor([], 100, 0), null)
  assert.deepEqual(newRowsBeforeAnchor(['a'], ['b'], 'a'), [])
  assert.deepEqual(newRowsBeforeAnchor(['a'], ['a', 'older-page'], 'a'), [])
  assert.notEqual(mailRowKey({ threadId: 'same', accountId: 'one' }), mailRowKey({ threadId: 'same', accountId: 'two' }))
  assert.equal(mailRowKey({ threadId: 'same' }, 'one'), mailRowKey({ threadId: 'same', accountId: 'one' }))
})

test('message list wiring preserves selection, disables native double-anchoring, and removes fixed-scroll sync restores', async () => {
  const app = await readFile(new URL('../src/lib/components/list/MessageList.svelte', import.meta.url), 'utf8')
  const jump = app.slice(app.indexOf('  function showNewestMail()'), app.indexOf('  function loadNextPage()'))
  assert.doesNotMatch(jump, /selectedThreadId\s*=/)
  assert.match(jump, /scrollTo\(\{ top: 0/)
  assert.match(app, /overflow-anchor: none/)
  const sync = app.slice(app.indexOf('  function scheduleReload()'), app.indexOf('  // Listen for folder sync'))
  assert.doesNotMatch(sync, /scrollTop\s*=/)
})


test('arrival burst pushes anchor beyond the first page: extend the window, bounded and deduplicated', async () => {
  let calls = 0
  const pages = [['new2', 'open', 'older'], []]
  const result = await extendAnchorWindow(['new1', 'new2'], 'open', row => row, async () => { calls++; return pages.shift() })
  assert.deepEqual(result, ['new1', 'new2', 'open', 'older'])
  assert.equal(calls, 1)
  calls = 0
  await extendAnchorWindow(['new'], 'deleted', row => row, async () => [`page${calls++}`])
  assert.equal(calls, 5, 'missing row never causes unbounded mailbox reads')
  await extendAnchorWindow(['open'], 'open', row => row, async () => { throw new Error('must not fetch') })
})
