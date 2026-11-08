import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/components/viewer/deduplicateMessages.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { deduplicateMessages } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

test('deduplicates multiple copies sharing the same RFC messageId', () => {
  const messages = [
    { id: 'msg-1', rfcMessageId: '<test-1@example.com>', folderId: 'all-mail', bodyFetched: true, date: '2026-11-08T12:00:00Z' },
    { id: 'msg-2', messageId: '<test-1@example.com>', folderId: 'inbox', bodyFetched: true, date: '2026-11-08T12:00:00Z' },
    { id: 'msg-3', messageId: 'test-1@example.com', folderId: 'all-mail', bodyFetched: false, date: '2026-11-08T12:00:00Z' },
    { id: 'msg-4', rfcMessageId: '<reply-1@example.com>', folderId: 'sent', bodyFetched: true, date: '2026-11-08T12:01:00Z' },
  ]

  const deduped = deduplicateMessages(messages, 'inbox')
  assert.equal(deduped.length, 2, 'should reduce to 2 unique messages (1 incoming + 1 reply)')
  assert.equal(deduped[0].id, 'msg-2', 'should pick the copy in the active inbox folder')
  assert.equal(deduped[1].id, 'msg-4', 'should retain distinct reply message')
})

test('prefers body-fetched copy when neither is in active folder', () => {
  const messages = [
    { id: 'msg-empty', messageId: '<test-2@example.com>', folderId: 'all-mail', bodyFetched: false },
    { id: 'msg-full', messageId: '<test-2@example.com>', folderId: 'all-mail', bodyFetched: true },
  ]

  const deduped = deduplicateMessages(messages, 'inbox')
  assert.equal(deduped.length, 1)
  assert.equal(deduped[0].id, 'msg-full')
})

test('returns input unmodified if empty or single message', () => {
  assert.deepEqual(deduplicateMessages([]), [])
  const single = [{ id: 'msg-single', messageId: '<single@example.com>' }]
  assert.deepEqual(deduplicateMessages(single), single)
})
