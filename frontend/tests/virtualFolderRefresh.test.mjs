import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/components/list/virtualFolderRefresh.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { refreshVirtualView, isExpectedSearchCancellation } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
test('manual refresh recomputes index before resetting cursor and loading live results', async () => {
  const calls = []
  await refreshVirtualView({ refreshCache: async () => calls.push('cache'), isCurrent: () => true, reset: () => calls.push('reset'), reload: async () => calls.push('load') })
  assert.deepEqual(calls, ['cache', 'reset', 'load'])
})
test('navigation during index refresh cannot reset or reload the new folder', async () => {
  const calls = []
  await refreshVirtualView({ refreshCache: async () => calls.push('cache'), isCurrent: () => false, reset: () => calls.push('reset'), reload: async () => calls.push('load') })
  assert.deepEqual(calls, ['cache'])
})
test('index failure preserves current rows and propagates a genuine error', async () => {
  let reset = false
  await assert.rejects(refreshVirtualView({ refreshCache: async () => { throw new Error('disk unavailable') }, isCurrent: () => true, reset: () => { reset = true }, reload: async () => {} }), /disk unavailable/)
  assert.equal(reset, false)
})
test('only known cancellation and supersession are normal outcomes', () => {
  for (const err of ['context canceled', 'context cancelled', 'search superseded', new DOMException('aborted', 'AbortError')]) assert.equal(isExpectedSearchCancellation(err), true)
  for (const err of ['Gmail quota exceeded', 'permission denied', 'unknown error', null]) assert.equal(isExpectedSearchCancellation(err), false)
})
