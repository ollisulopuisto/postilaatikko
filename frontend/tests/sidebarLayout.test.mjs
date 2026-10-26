// P-008: sidebar block order and visibility.
//
// The sidebar is made of blocks (unified inbox, pinned folders, virtual
// folders, calendar, special folders, other folders). A layout is the order of
// those blocks plus the set that is hidden. Persisted data can be stale or
// partial, so normalization must always produce a complete, valid layout.

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

const {
  SIDEBAR_BLOCKS,
  DEFAULT_LAYOUT,
  normalizeLayout,
  moveBlock,
  setBlockHidden,
  visibleBlocks,
  blockPosition,
} = await load('../src/lib/sidebar/layout.ts')

test('the default layout lists every block in the original order, none hidden', () => {
  assert.deepEqual(DEFAULT_LAYOUT.order, ['unified', 'pinned', 'virtual', 'calendar', 'primary', 'secondary'])
  assert.deepEqual(DEFAULT_LAYOUT.hidden, [])
  assert.deepEqual([...SIDEBAR_BLOCKS], DEFAULT_LAYOUT.order)
})

test('normalizeLayout returns the default for missing or malformed input', () => {
  const cases = [undefined, null, 'virtual', 42, {}]
  for (const raw of cases) {
    assert.deepEqual(normalizeLayout(raw, undefined), DEFAULT_LAYOUT, `input ${JSON.stringify(raw)}`)
  }
})

test('normalizeLayout drops unknown and duplicate blocks and appends missing ones', () => {
  const layout = normalizeLayout(['virtual', 'bogus', 'virtual', 'unified'], undefined)
  assert.deepEqual(layout.order, ['virtual', 'unified', 'pinned', 'calendar', 'primary', 'secondary'])
})

test('normalizeLayout keeps a valid hidden set and ignores unknown hidden entries', () => {
  const layout = normalizeLayout(undefined, ['calendar', 'nope', 'calendar'])
  assert.deepEqual(layout.hidden, ['calendar'])
})

test('moveBlock swaps a block with its neighbour in the chosen direction', () => {
  const moved = moveBlock(DEFAULT_LAYOUT, 'pinned', -1)
  assert.deepEqual(moved.order, ['pinned', 'unified', 'virtual', 'calendar', 'primary', 'secondary'])
  const back = moveBlock(moved, 'pinned', 1)
  assert.deepEqual(back.order, DEFAULT_LAYOUT.order)
})

test('moveBlock does nothing past either end or for an unknown block', () => {
  assert.deepEqual(moveBlock(DEFAULT_LAYOUT, 'unified', -1), DEFAULT_LAYOUT)
  assert.deepEqual(moveBlock(DEFAULT_LAYOUT, 'secondary', 1), DEFAULT_LAYOUT)
  assert.deepEqual(moveBlock(DEFAULT_LAYOUT, 'bogus', 1), DEFAULT_LAYOUT)
})

test('setBlockHidden hides and shows a block without changing the order', () => {
  const hidden = setBlockHidden(DEFAULT_LAYOUT, 'virtual', true)
  assert.deepEqual(hidden.hidden, ['virtual'])
  assert.deepEqual(hidden.order, DEFAULT_LAYOUT.order)
  const shown = setBlockHidden(hidden, 'virtual', false)
  assert.deepEqual(shown.hidden, [])
})

test('visibleBlocks keeps the order and skips hidden blocks', () => {
  const moved = moveBlock(moveBlock(DEFAULT_LAYOUT, 'calendar', -1), 'calendar', -1)
  const layout = setBlockHidden(moved, 'unified', true)
  assert.deepEqual(visibleBlocks(layout), ['calendar', 'pinned', 'virtual', 'primary', 'secondary'])
})

test('blockPosition reports the index in the order, including hidden blocks', () => {
  const layout = setBlockHidden(DEFAULT_LAYOUT, 'pinned', true)
  assert.equal(blockPosition(layout, 'unified'), 0)
  assert.equal(blockPosition(layout, 'pinned'), 1)
  assert.equal(blockPosition(layout, 'secondary'), 5)
})

test('layout operations do not mutate their input', () => {
  const snapshot = JSON.stringify(DEFAULT_LAYOUT)
  moveBlock(DEFAULT_LAYOUT, 'pinned', -1)
  setBlockHidden(DEFAULT_LAYOUT, 'virtual', true)
  assert.equal(JSON.stringify(DEFAULT_LAYOUT), snapshot)
})
