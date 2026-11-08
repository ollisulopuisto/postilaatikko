import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function loadThreadScope() {
  const source = await readFile(new URL('../src/lib/components/viewer/threadScope.ts', import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}

const { preserveThreadMessageScope } = await loadThreadScope()

test('body refresh keeps the original thread-scope marker', () => {
  const original = { id: 'context', matched: false, bodyFetched: false }
  const fetched = { id: 'context', bodyFetched: true, bodyText: 'body' }
  assert.deepEqual(preserveThreadMessageScope(original, fetched), {
    id: 'context',
    matched: false,
    bodyFetched: true,
    bodyText: 'body',
  })
})

test('local messages without a service scope marker stay unmarked', () => {
  const original = { id: 'local' }
  assert.deepEqual(preserveThreadMessageScope(original, { id: 'local', bodyFetched: true }), {
    id: 'local',
    matched: undefined,
    bodyFetched: true,
  })
})
