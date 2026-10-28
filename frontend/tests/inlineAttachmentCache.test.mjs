import { test, beforeEach } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../src/lib/stores/inlineAttachmentCache.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const cache = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const budget = 32 * 1024 * 1024
const image = (length) => ({ image: 'x'.repeat(length) })
beforeEach(() => cache.clearCache())

test('keeps API behavior and advances the invalidation generation on clear', () => {
  const data = image(10)
  const generation = cache.getCacheGeneration()
  assert.equal(cache.getCached('missing'), null)
  cache.setCache('a', data)
  assert.equal(cache.getCached('a'), data)
  cache.clearCache()
  assert.equal(cache.getCacheSize(), 0)
  assert.equal(cache.getCached('a'), null)
  assert.equal(cache.getCacheGeneration(), generation + 1)
})

test('evicts least recently read entries at the 128-message limit', () => {
  for (let i = 0; i < 128; i++) cache.setCache(String(i), image(10))
  cache.getCached('0')
  cache.setCache('128', image(10))
  assert.equal(cache.getCacheSize(), 128)
  assert.notEqual(cache.getCached('0'), null)
  assert.equal(cache.getCached('1'), null)
})

test('bounds retained string payloads, including keys, at 32 MiB', () => {
  // Each value is 4 MiB in the conservative two-bytes-per-code-unit accounting.
  for (let i = 0; i < 12; i++) cache.setCache(String(i), image(2 * 1024 * 1024))
  assert.ok(cache.getCacheSize() <= 7)
  assert.equal(cache.getCached('0'), null)
  assert.notEqual(cache.getCached('11'), null)
})

test('skips oversized entries without evicting useful smaller entries', () => {
  cache.setCache('small', image(10))
  cache.setCache('large', image(budget / 2))
  assert.ok(cache.getCached('large') === null)
  assert.notEqual(cache.getCached('small'), null)
})

test('replacing an entry updates accounting without double counting', () => {
  for (let i = 0; i < 12; i++) cache.setCache('same', image(2 * 1024 * 1024))
  assert.equal(cache.getCacheSize(), 1)
  cache.setCache('other', image(2 * 1024 * 1024))
  assert.equal(cache.getCacheSize(), 2)
  cache.setCache('same', image(budget / 2))
  assert.ok(cache.getCached('same') === null)
  assert.notEqual(cache.getCached('other'), null)
})

test('clear resets accounting and eviction does not change invalidation generation', () => {
  const generation = cache.getCacheGeneration()
  for (let i = 0; i < 140; i++) cache.setCache(String(i), image(10))
  assert.equal(cache.getCacheGeneration(), generation)
  cache.clearCache()
  cache.setCache('fresh', image(10))
  assert.equal(cache.getCacheSize(), 1)
  assert.notEqual(cache.getCached('fresh'), null)
})

test('large message IDs and content IDs count towards the payload budget', () => {
  cache.setCache('a'.repeat(budget / 2), image(1))
  cache.setCache('b', { ['b'.repeat(budget / 2)]: 'x' })
  assert.equal(cache.getCacheSize(), 0)
})
