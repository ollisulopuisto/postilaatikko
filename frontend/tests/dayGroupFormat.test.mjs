// Bolt optimization regression: month formatting in the production dayGroupFor
// (src/lib/components/list/dayGroup.ts), used by MessageList.svelte.
//
// This test transpiles and imports the real production module - no copied
// implementation - so behavior and caching regressions fail here directly.
//
// Covers:
// 1. grouping correctness across dates and locales (pinned TZ=UTC),
// 2. month-boundary dates across a system time-zone change (Devin finding:
//    a formatter cached under the old zone formats the label in that zone
//    while the group id uses the new zone),
// 3. a >5x performance guard over constructing Intl.DateTimeFormat per call.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

process.env.TZ = 'UTC'

const source = await readFile(new URL('../src/lib/components/list/dayGroup.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } })
const { dayGroupFor } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const labels = { today: 'Today', yesterday: 'Yesterday' }

test('dayGroupFor groups fixed dates by month with year (TZ=UTC)', () => {
  // The year suffix appears only when the date's year differs from today's,
  // so derive the expectation from the current year instead of hardcoding it:
  // from 2027 on, March 2026 is labelled "March 2026", not "March".
  const thisYear = new Date().getFullYear()
  const expectLabel = (month, year) => (year === thisYear ? month : `${month} ${year}`)
  const cases = [
    ['2026-03-15T12:00:00Z', 'month-2026-2', expectLabel('March', 2026)],
    ['2025-11-20T08:30:00Z', 'month-2025-10', expectLabel('November', 2025)],
    ['2024-02-29T14:15:00Z', 'month-2024-1', expectLabel('February', 2024)],
    ['2023-01-01T00:00:00Z', 'month-2023-0', expectLabel('January', 2023)],
    ['2022-07-04T12:00:00Z', 'month-2022-6', expectLabel('July', 2022)],
  ]
  for (const [date, id, label] of cases) {
    assert.deepEqual(dayGroupFor(date, 'en', labels), { id, label }, `date=${date}`)
  }
})

test('dayGroupFor labels today and yesterday', () => {
  assert.equal(dayGroupFor(new Date(), 'en', labels).id, 'today')
  assert.equal(dayGroupFor(new Date(), 'en', labels).label, 'Today')
  assert.equal(dayGroupFor(new Date(Date.now() - 86400000), 'en', labels).id, 'yesterday')
})

test('dayGroupFor produces consistent groups across locales', () => {
  for (const lang of ['en', 'fi', 'de', 'fr', 'pt-BR', 'zh-CN', '']) {
    const result = dayGroupFor('2025-11-20T08:30:00Z', lang, labels)
    assert.equal(result.id, 'month-2025-10', `lang=${lang}`)
    assert.ok(result.label.length > 0, `lang=${lang} label must not be empty`)
  }
})

test('month headings follow a system time-zone change on a month-boundary date', () => {
  const previousTZ = process.env.TZ
  try {
    // 2025-04-01T00:30:00Z is March 31 17:30 in Los Angeles but April 1 03:30
    // in Helsinki. A formatter cached under the old zone must not format the
    // label after the zone changes: id and label must agree on the month.
    process.env.TZ = 'America/Los_Angeles'
    const before = dayGroupFor('2025-04-01T00:30:00Z', 'en', labels)
    assert.equal(before.id, 'month-2025-2')
    assert.match(before.label, /^March/)

    process.env.TZ = 'Europe/Helsinki'
    const after = dayGroupFor('2025-04-01T00:30:00Z', 'en', labels)
    assert.equal(after.id, 'month-2025-3')
    assert.match(after.label, /^April/, `label "${after.label}" must come from the active zone, not a formatter cached under the previous one`)
  } finally {
    process.env.TZ = previousTZ
  }
})

// ---- Performance regression guard ----
function dayGroupForUncached(dateValue, lang) {
  const date = dateValue ? new Date(dateValue) : new Date(0)
  const month = new Intl.DateTimeFormat(lang || 'en', { month: 'long' }).format(date)
  const monthLabel = `${month.charAt(0).toUpperCase()}${month.slice(1)}`
  return { id: `month-${date.getFullYear()}-${date.getMonth()}`, label: monthLabel }
}

function bench(fn, iterations) {
  for (let i = 0; i < 50; i++) fn() // warmup
  const start = process.hrtime.bigint()
  for (let i = 0; i < iterations; i++) fn()
  const end = process.hrtime.bigint()
  return Number(end - start) / 1_000_000 // ms
}

const ITERATIONS = 200
const olderDates = [
  '2026-03-15T12:00:00Z',
  '2025-11-20T08:30:00Z',
  '2024-02-29T14:15:00Z',
  '2023-01-01T00:00:00Z',
]

test('dayGroupFor cached formatter is materially faster than uncached (>5x speedup)', () => {
  const runOld = () => {
    for (const d of olderDates) {
      for (const l of ['fi', 'en']) {
        dayGroupForUncached(d, l)
      }
    }
  }
  const runNew = () => {
    for (const d of olderDates) {
      for (const l of ['fi', 'en']) {
        dayGroupFor(d, l, labels)
      }
    }
  }

  const oldMs = bench(runOld, ITERATIONS)
  const newMs = bench(runNew, ITERATIONS)

  console.log(`dayGroupFor uncached=${oldMs.toFixed(2)}ms cached=${newMs.toFixed(2)}ms over ${ITERATIONS} iterations`)
  assert.ok(newMs < oldMs, `expected cached (${newMs}ms) < uncached (${oldMs}ms)`)
  const speedup = oldMs / newMs
  assert.ok(speedup > 5, `expected >5x speedup, got ${speedup.toFixed(2)}x`)
})
