// Invalid header dates must never break chronological inbox grouping.
// A half-synced old message can arrive with an empty or unparsable
// latestDate; dayGroupFor previously let Intl.DateTimeFormat.format throw
// "Invalid time value" inside the message-list render.
//
// Transpiles and imports the real production module - no copied
// implementation - so a regression here fails directly.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

process.env.TZ = 'UTC'

const source = await readFile(new URL('../src/lib/components/list/dayGroup.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } })
const { dayGroupFor } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const labels = { today: 'Tänään', yesterday: 'Eilen' }

test('dayGroupFor buckets invalid dates with the oldest mail without throwing', () => {
  const epoch = dayGroupFor(undefined, 'fi', labels)
  for (const bad of ['', 'not-a-date', '2026-13-40T99:99:99Z', NaN, null]) {
    let group
    assert.doesNotThrow(() => { group = dayGroupFor(bad, 'fi', labels) }, `throws for ${String(bad)}`)
    assert.equal(group.id, epoch.id, `invalid date ${String(bad)} should share the epoch bucket`)
    assert.equal(group.label, epoch.label)
  }
})

test('dayGroupFor still groups valid dates normally', () => {
  const now = new Date()
  assert.equal(dayGroupFor(now.toISOString(), 'fi', labels).id, 'today')
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1)
  assert.equal(dayGroupFor(yesterday.toISOString(), 'fi', labels).id, 'yesterday')
  const old = dayGroupFor('2026-11-14T10:00:00Z', 'fi', labels)
  assert.ok(old.id.startsWith('month-'), `expected a month bucket, got ${old.id}`)
})
