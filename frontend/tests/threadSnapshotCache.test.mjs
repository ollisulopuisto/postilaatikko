import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../src/lib/components/viewer/threadSnapshotCache.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { getThreadSnapshot, setThreadSnapshot, clearThreadSnapshots } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

function conversation(id, bodyLength = 0) {
  return {
    threadId: id,
    subject: id,
    snippet: '',
    messageCount: 1,
    unreadCount: 0,
    hasAttachments: false,
    isStarred: false,
    latestDate: null,
    participants: [],
    messageIds: [id],
    isEncrypted: false,
    messages: [{ id, subject: id, snippet: '', bodyText: 'x'.repeat(bodyLength), bodyHtml: '', fromName: '', fromEmail: '', toList: '', ccList: '', bccList: '', replyTo: '' }],
  }
}

test('thread snapshots are shared by account, folder, and thread and evict least-recently-used', () => {
  clearThreadSnapshots()
  for (let index = 1; index <= 6; index += 1) setThreadSnapshot('account', 'folder', `thread-${index}`, conversation(`thread-${index}`))
  assert.equal(getThreadSnapshot('account', 'folder', 'thread-1')?.subject, 'thread-1')
  setThreadSnapshot('account', 'folder', 'thread-7', conversation('thread-7'))

  assert.equal(getThreadSnapshot('account', 'folder', 'thread-2'), null)
  assert.equal(getThreadSnapshot('account', 'folder', 'thread-1')?.subject, 'thread-1')
  assert.equal(getThreadSnapshot('account', 'folder', 'thread-7')?.subject, 'thread-7')
})

test('thread snapshots do not cross account or folder boundaries', () => {
  clearThreadSnapshots()
  setThreadSnapshot('account-1', 'folder-1', 'thread-1', conversation('thread-1'))

  assert.equal(getThreadSnapshot('account-2', 'folder-1', 'thread-1'), null)
  assert.equal(getThreadSnapshot('account-1', 'folder-2', 'thread-1'), null)
  assert.equal(getThreadSnapshot('account-1', 'folder-1', 'thread-1')?.subject, 'thread-1')
})

test('thread snapshots are bounded by estimated content size', () => {
  clearThreadSnapshots()
  setThreadSnapshot('account', 'folder', 'large-1', conversation('large-1', 1_200_000))
  setThreadSnapshot('account', 'folder', 'large-2', conversation('large-2', 1_200_000))

  assert.equal(getThreadSnapshot('account', 'folder', 'large-1'), null)
  assert.equal(getThreadSnapshot('account', 'folder', 'large-2')?.subject, 'large-2')
})

test('thread snapshot estimate includes reference headers and message ids', () => {
  clearThreadSnapshots()
  const referenceHeavy = conversation('reference-heavy', 1_200_000)
  referenceHeavy.messages[0].references = 'r'.repeat(1_000_000)
  referenceHeavy.messageIds = Array.from({ length: 100 }, (_, index) => `message-id-${index}-` + 'm'.repeat(100))
  setThreadSnapshot('account', 'folder', 'reference-heavy', referenceHeavy)

  assert.equal(getThreadSnapshot('account', 'folder', 'reference-heavy'), null)
})

test('oversized threads are not retained', () => {
  clearThreadSnapshots()
  setThreadSnapshot('account', 'folder', 'huge', conversation('huge', 3_000_000))
  setThreadSnapshot('account', 'folder', 'small', conversation('small'))

  assert.equal(getThreadSnapshot('account', 'folder', 'huge'), null)
  assert.equal(getThreadSnapshot('account', 'folder', 'small')?.subject, 'small')
})
