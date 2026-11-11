// Bolt optimization regression: month formatting in MessageList.svelte `dayGroupFor`.
//
// OLD (previous source): `new Intl.DateTimeFormat(getLanguage() || 'en', { month: 'long' })`
// is instantiated on every older conversation row during chronological inbox listing.
//
// NEW: `Intl.DateTimeFormat` instances are cached in a Map keyed by language.
//
// This test verifies:
// 1. old and new implementations produce identical outputs across various dates and locales.
// 2. new is materially faster than old (>5x performance regression guard).

import { test } from 'node:test'
import assert from 'node:assert/strict'

// ---- OLD implementation (instantiating new Intl.DateTimeFormat per call) ----
function dayGroupForOld(dateValue, lang) {
  const date = dateValue ? new Date(dateValue) : new Date(0)
  const today = new Date()
  const startToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()
  const startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1).getTime()
  if (startDate === startToday) return { id: 'today', label: 'today' }
  if (startDate === yesterday) return { id: 'yesterday', label: 'yesterday' }
  const sameYear = date.getFullYear() === today.getFullYear()
  const month = new Intl.DateTimeFormat(lang || 'en', { month: 'long' }).format(date)
  const monthLabel = `${month.charAt(0).toUpperCase()}${month.slice(1)}`
  return {
    id: `month-${date.getFullYear()}-${date.getMonth()}`,
    label: sameYear ? monthLabel : `${monthLabel} ${date.getFullYear()}`,
  }
}

// ---- NEW implementation (cached formatter per language) ----
const sharedMonthFormatterCache = new Map()

function dayGroupForNew(dateValue, lang, cache = sharedMonthFormatterCache) {
  const date = dateValue ? new Date(dateValue) : new Date(0)
  const today = new Date()
  const startToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()
  const startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1).getTime()
  if (startDate === startToday) return { id: 'today', label: 'today' }
  if (startDate === yesterday) return { id: 'yesterday', label: 'yesterday' }
  const sameYear = date.getFullYear() === today.getFullYear()
  const l = lang || 'en'
  let formatter = cache.get(l)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(l, { month: 'long' })
    cache.set(l, formatter)
  }
  const month = formatter.format(date)
  const monthLabel = `${month.charAt(0).toUpperCase()}${month.slice(1)}`
  return {
    id: `month-${date.getFullYear()}-${date.getMonth()}`,
    label: sameYear ? monthLabel : `${monthLabel} ${date.getFullYear()}`,
  }
}

const testDates = [
  new Date(),
  new Date(Date.now() - 86400000), // yesterday
  new Date(Date.now() - 86400000 * 30), // ~1 month ago
  new Date('2026-03-15T12:00:00Z'),
  new Date('2025-11-20T08:30:00Z'),
  new Date('2024-02-29T14:15:00Z'),
  '2023-01-01T00:00:00Z',
  '2022-07-04T12:00:00Z',
  undefined,
]

const testLangs = ['en', 'fi', 'de', 'fr', 'pt-BR', 'zh-CN', '']

test('dayGroupFor old and new produce identical output across dates and locales', () => {
  for (const date of testDates) {
    for (const lang of testLangs) {
      const oldRes = dayGroupForOld(date, lang)
      const newRes = dayGroupForNew(date, lang)
      assert.deepEqual(newRes, oldRes, `mismatch for date=${date} lang=${lang}`)
    }
  }
})

test('dayGroupFor cached formatter caches per locale and reuses instance', () => {
  const localCache = new Map()
  assert.equal(localCache.size, 0)
  dayGroupForNew('2025-01-01T00:00:00Z', 'fi', localCache)
  assert.equal(localCache.size, 1)
  dayGroupForNew('2025-02-01T00:00:00Z', 'fi', localCache)
  assert.equal(localCache.size, 1)
  dayGroupForNew('2025-03-01T00:00:00Z', 'en', localCache)
  assert.equal(localCache.size, 2)
})

// ---- Performance regression guard ----
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

test('dayGroupFor cached formatter is materially faster than old (>5x speedup)', () => {
  const runOld = () => {
    for (const d of olderDates) {
      for (const l of ['fi', 'en']) {
        dayGroupForOld(d, l)
      }
    }
  }

  const localCache = new Map()
  const runNew = () => {
    for (const d of olderDates) {
      for (const l of ['fi', 'en']) {
        dayGroupForNew(d, l, localCache)
      }
    }
  }

  const oldMs = bench(runOld, ITERATIONS)
  const newMs = bench(runNew, ITERATIONS)

  console.log(`dayGroupFor old=${oldMs.toFixed(2)}ms new=${newMs.toFixed(2)}ms over ${ITERATIONS} iterations`)
  assert.ok(newMs < oldMs, `expected new (${newMs}ms) < old (${oldMs}ms)`)
  const speedup = oldMs / newMs
  assert.ok(speedup > 5, `expected >5x speedup, got ${speedup.toFixed(2)}x`)
})
