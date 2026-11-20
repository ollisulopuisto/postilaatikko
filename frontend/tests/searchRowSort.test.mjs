// Production search-row correctness and deterministic parse-work budget.
// Wall-clock ratios on shared runners are not a reproducible regression guard.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

// Reference: current pre-optimisation logic (parses per comparison).
function sortSearchRowsDate(rows, direction) {
  return [...rows].sort((a, b) => direction * (new Date(a.latestDate).getTime() - new Date(b.latestDate).getTime()))
}

// Exercise the production handler, not a separately maintained copy.
const component = readFileSync(new URL('../src/lib/components/list/MessageList.svelte', import.meta.url), 'utf8')
const start = component.indexOf('  function sortSearchRows(')
const end = component.indexOf('  export function startAllMailSearch(', start)
assert.ok(start >= 0 && end > start, 'production sort handler boundaries must exist')
const production = ts.transpileModule(component.slice(start, end), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
function productionSort(rows, direction, DateClass = Date) {
  const context = { searchSort: direction === 1 ? 'oldest' : 'newest', Date: DateClass, rows }
  runInNewContext(production + '\nresult = sortSearchRows(rows)', context)
  return Array.from(context.result)
}
const sortSearchRows = productionSort

function isoRows(base, count, stepMs) {
  const rows = []
  for (let i = 0; i < count; i++) {
    rows.push({ latestDate: new Date(base.getTime() + i * stepMs).toISOString(), id: String(i) })
  }
  return rows
}

const base = new Date(Date.UTC(2026, 10, 18, 12, 0, 0))

test('sortSearchRows orders newest-first matching Date.getTime()', () => {
  const rows = isoRows(base, 50, 3600000)
  const expected = sortSearchRowsDate(rows, -1).map((r) => r.id)
  const actual = sortSearchRows(rows, -1).map((r) => r.id)
  assert.deepEqual(actual, expected)
})

test('sortSearchRows orders oldest-first matching Date.getTime()', () => {
  const rows = isoRows(base, 50, 3600000)
  const expected = sortSearchRowsDate(rows, 1).map((r) => r.id)
  const actual = sortSearchRows(rows, 1).map((r) => r.id)
  assert.deepEqual(actual, expected)
})

test('sortSearchRows orders mixed-precision same-second timestamps newest-first', () => {
  // Go time.Time JSON drops a zero fraction: 05Z is 05.000, before 05.123.
  const rows = [
    { latestDate: '2026-11-18T12:00:05Z', id: 'whole' },
    { latestDate: '2026-11-18T12:00:05.123Z', id: 'fraction' },
  ]
  const actual = sortSearchRows(rows, -1).map((r) => r.id)
  assert.deepEqual(actual, ['fraction', 'whole'])
})

test('sortSearchRows orders mixed-precision same-second timestamps oldest-first', () => {
  const rows = [
    { latestDate: '2026-11-18T12:00:05.123Z', id: 'fraction' },
    { latestDate: '2026-11-18T12:00:05Z', id: 'whole' },
  ]
  const actual = sortSearchRows(rows, 1).map((r) => r.id)
  assert.deepEqual(actual, ['whole', 'fraction'])
})

test('sortSearchRows matches Date.getTime() ordering for mixed-precision sets', () => {
  const rows = [
    { latestDate: '2026-11-18T12:00:05Z', id: 'a' },
    { latestDate: '2026-11-18T12:00:05.999Z', id: 'b' },
    { latestDate: '2026-11-18T12:00:04.5Z', id: 'c' },
    { latestDate: '2026-11-18T12:00:06Z', id: 'd' },
    { latestDate: '2026-11-18T12:00:04.500Z', id: 'e' },
  ]
  for (const direction of [-1, 1]) {
    const expected = sortSearchRowsDate(rows, direction).map((r) => r.id)
    const actual = sortSearchRows(rows, direction).map((r) => r.id)
    assert.deepEqual(actual, expected)
  }
})

test('sortSearchRows pins unparseable dates last in both directions', () => {
  // Date.parse returns NaN on malformed input; NaN in a numeric comparator
  // would order the row inconsistently, so bad rows always sort last.
  const rows = [
    { latestDate: 'not-a-date', id: 'bad' },
    { latestDate: '2026-11-18T12:00:05Z', id: 'mid' },
    { latestDate: '', id: 'empty' },
    { latestDate: '2026-11-18T12:00:06Z', id: 'new' },
    { latestDate: '2026-11-18T12:00:04Z', id: 'old' },
  ]
  assert.deepEqual(sortSearchRows(rows, -1).map((r) => r.id), ['new', 'mid', 'old', 'bad', 'empty'])
  assert.deepEqual(sortSearchRows(rows, 1).map((r) => r.id), ['old', 'mid', 'new', 'bad', 'empty'])
})

test('sortSearchRows is stable for equal dates', () => {
  const rows = [
    { latestDate: '2026-11-18T12:00:00.000Z', id: 'a' },
    { latestDate: '2026-11-18T12:00:00.000Z', id: 'b' },
    { latestDate: '2026-11-18T12:00:00.000Z', id: 'c' },
  ]
  const actual = sortSearchRows(rows, -1).map((r) => r.id)
  assert.deepEqual(actual, ['a', 'b', 'c'])
})

test('sortSearchRows handles a single row and empty input', () => {
  assert.deepEqual(sortSearchRows([{ latestDate: '2026-11-18T12:00:00.000Z', id: 'a' }], -1).map((r) => r.id), ['a'])
  assert.deepEqual(sortSearchRows([], -1), [])
})

// Guard the expensive work directly: every timestamp is read and parsed once,
// whether rows arrive in sorted, reversed or shuffled order.
for (const order of ['sorted', 'reversed', 'shuffled']) {
  test(`production sort parses each date once for ${order} input`, () => {
    let parses = 0, reads = 0
    class CountingDate extends Date {
      constructor(...args) { super(...args); parses++ }
      static parse(value) { parses++; return Date.parse(value) }
    }
    const rows = isoRows(base, 500, 60000)
    if (order === 'reversed') rows.reverse()
    if (order === 'shuffled') {
      let state = 17
      for (let i = rows.length - 1; i > 0; i--) {
        state = (state * 1664525 + 1013904223) >>> 0
        const j = state % (i + 1)
        ;[rows[i], rows[j]] = [rows[j], rows[i]]
      }
    }
    for (const row of rows) {
      const date = row.latestDate
      Object.defineProperty(row, 'latestDate', { get() { reads++; return date } })
    }
    const actual = productionSort(rows, -1, CountingDate)
    assert.equal(parses, rows.length, 'one parse per row, not per comparison')
    assert.equal(reads, rows.length, 'one timestamp read per row')
    assert.deepEqual(actual.map(r => r.id), Array.from({ length: 500 }, (_, i) => String(499 - i)))
    assert.equal(rows.length, 500)
  })
}
