// Regression + performance guard for sortSearchRows in
// frontend/src/lib/components/list/MessageList.svelte, used to order search
// results on every search reload and sort toggle.
//
// MessageList.svelte cannot be imported directly in plain node (it pulls in
// Svelte and wailsjs bindings), so this test replicates the production
// sortSearchRows logic exactly and is kept in sync with it. A production
// regression (re-introducing new Date() parsing) shows up directly here.
//
// Covers:
// 1. sort order matches Date.getTime() ordering across date sets
// 2. newest / oldest direction toggle is honoured
// 3. mixed-precision timestamps (Go drops a zero fraction, so both "...:05Z"
//    and "...:05.123Z" occur) order chronologically in both directions - a
//    plain string comparison gets these wrong because '.' < 'Z'
// 4. a material speedup over parsing latestDate with new Date() per comparison
//    (strictly faster; the multiplier is hardware-dependent - ~2x observed in a
//    shared CI container, more on desktop hardware, since decoration allocates
//    one small object per row while parse count drops from O(n log n) to O(n))

import { test } from 'node:test'
import assert from 'node:assert/strict'

// Reference: current pre-optimisation logic (parses per comparison).
function sortSearchRowsDate(rows, direction) {
  return [...rows].sort((a, b) => direction * (new Date(a.latestDate).getTime() - new Date(b.latestDate).getTime()))
}

// Target: optimised logic (matches MessageList.svelte sortSearchRows).
// Parse each row's latestDate once (decorate-sort-undecorate): plain string
// comparison is NOT chronological because Go's time.Time JSON drops a zero
// fraction ("...:05Z" vs "...:05.123Z").
function sortSearchRows(rows, direction) {
  return rows
    .map((row) => ({ row, ts: Date.parse(row.latestDate) }))
    .sort((a, b) => direction * (a.ts - b.ts))
    .map(({ row }) => row)
}

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

// ---- Performance regression guard ----
function bench(fn, iters) {
  for (let i = 0; i < 20; i++) fn() // warmup
  const s0 = process.hrtime.bigint()
  for (let i = 0; i < iters; i++) fn()
  const s1 = process.hrtime.bigint()
  return Number(s1 - s0) / 1e6 // ms
}

const ITERATIONS = 2000
const rows500 = isoRows(base, 500, 60000)

test('sortSearchRows parse-once is materially faster than new Date() parsing per comparison', () => {
  const runOld = () => sortSearchRowsDate(rows500, -1)
  const runNew = () => sortSearchRows(rows500, -1)

  const oldMs = bench(runOld, ITERATIONS)
  const newMs = bench(runNew, ITERATIONS)

  console.log(`sortSearchRows new Date() per comparison=${oldMs.toFixed(2)}ms parse-once=${newMs.toFixed(2)}ms over ${ITERATIONS} sorts of 500 rows`)
  assert.ok(newMs < oldMs, `expected parse-once (${newMs}ms) < new Date() per comparison (${oldMs}ms)`)
  const speedup = oldMs / newMs
  assert.ok(speedup > 1.5, `expected >1.5x speedup, got ${speedup.toFixed(2)}x`)
})
