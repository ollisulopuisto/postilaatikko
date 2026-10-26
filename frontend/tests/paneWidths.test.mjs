// P-039: narrower default sidebar and message list, so the message gets more room.
//
// The new defaults apply once, on the first launch after the update, the same
// way the earlier sidebar layout migration did. Widths the user sets afterwards
// are kept.

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

const { PANE_WIDTH_DEFAULTS, applyNarrowerDefaults } = await load('../src/lib/layout/paneWidths.ts')

test('the new defaults are narrower than the old 320px sidebar and 360px list', () => {
  assert.deepEqual(PANE_WIDTH_DEFAULTS, { sidebarWidth: 260, listWidth: 320 })
})

test('the first launch after the update applies the narrower defaults once', () => {
  const before = { sidebarWidth: 320, listWidth: 360 }
  const first = applyNarrowerDefaults(before, false)
  assert.deepEqual(first.widths, { sidebarWidth: 260, listWidth: 320 })
  assert.equal(first.applied, true)
})

test('after the migration, widths the user sets are kept', () => {
  const custom = { sidebarWidth: 380, listWidth: 500 }
  const later = applyNarrowerDefaults(custom, true)
  assert.deepEqual(later.widths, custom)
  assert.equal(later.applied, true)
})

test('applyNarrowerDefaults does not mutate its input', () => {
  const input = { sidebarWidth: 320, listWidth: 360 }
  applyNarrowerDefaults(input, false)
  assert.deepEqual(input, { sidebarWidth: 320, listWidth: 360 })
})
