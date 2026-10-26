import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
const source = await readFile(new URL('../src/lib/components/viewer/bubbleLayout.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { bubbleLayout } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const message = (id, fromEmail) => ({ id, fromEmail })
test('all own identities use the right side; peers use left; consecutive sender is labelled once', () => {
  const result = bubbleLayout([message('a', 'peer@example.com'), message('b', 'PEER@example.com'), message('c', 'alias@example.com'), message('d', 'me@example.com')], email => ['me@example.com', 'alias@example.com'].includes(email))
  assert.equal(result.get('a').side, 'left')
  assert.equal(result.get('b').showSender, false)
  assert.equal(result.get('c').side, 'right')
  assert.equal(result.get('d').side, 'right')
})
test('two other participants keep stable sides; group threads retain sender labels', () => {
  const result = bubbleLayout([message('a', 'one@example.com'), message('b', 'two@example.com'), message('c', 'one@example.com')], () => false)
  assert.equal(result.get('b').side, 'right')
  assert.equal(result.get('c').side, 'left')
  assert.equal(result.get('c').showSender, true)
  const group = bubbleLayout([message('a', 'one'), message('b', 'two'), message('c', 'three')], () => false)
  assert.ok([...group.values()].every(value => value.side === 'left' && value.showSender))
})
