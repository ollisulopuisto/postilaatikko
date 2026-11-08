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

const reactionSource = await readFile(new URL('../src/lib/components/viewer/reactionLayout.ts', import.meta.url), 'utf8')
const reactionJS = ts.transpileModule(reactionSource, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { reactionLayout } = await import(`data:text/javascript;base64,${Buffer.from(reactionJS).toString('base64')}`)
const viewerSource = await readFile(new URL('../src/lib/components/viewer/ConversationViewer.svelte', import.meta.url), 'utf8')
const script = viewerSource.slice(viewerSource.indexOf('>', viewerSource.indexOf('<script')) + 1, viewerSource.indexOf('</script>'))
const viewer = ts.createSourceFile('viewer.ts', script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)

function actualReaderState(messages, mode, revealed = new Set()) {
  const names = new Set(['threadMessages', 'reactions', 'filteredMessages', 'readerMessages'])
  const statements = viewer.statements.filter(node => ts.isVariableStatement(node) && node.declarationList.declarations.some(declaration => names.has(declaration.name.getText(viewer))))
  assert.ok(statements.length >= 3, 'execute actual viewer pipeline, not a parallel implementation')
  const source = statements.map(node => node.getText(viewer)).join('\n')
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
  return new Function('conversation', 'folderId', 'readerMode', 'revealedReactions', '$derived', 'deduplicateMessages', 'reactionLayout', js + '\nreturn { readerMessages, reactions }')(
    { messages }, 'inbox', mode, revealed, value => value, deduplicateMessages, reactionLayout,
  )
}

function actualLastMessageId(readerMessages, messages, mode) {
  const handler = viewer.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === 'getLastMessageId')
  assert.ok(handler)
  const source = handler.getText(viewer).replace(/^export /, '')
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
  return new Function('readerMessages', 'conversation', 'readerMode', 'reactions', js + '\nreturn getLastMessageId()')(
    readerMessages, { messages }, mode, reactionLayout(messages),
  )
}

test('default reply/forward target is the last displayed copy in both reader modes', () => {
  const messages = [
    { id: 'visible', accountId: 'a', messageId: '<same@example.org>', folderId: 'inbox', bodyFetched: true },
    { id: 'hidden', accountId: 'a', messageId: '<same@example.org>', folderId: 'all', bodyFetched: false },
  ]
  for (const mode of ['traditional', 'conversation']) {
    const { readerMessages } = actualReaderState(messages, mode)
    assert.deepEqual(readerMessages.map(message => message.id), ['visible'])
    assert.equal(actualLastMessageId(readerMessages, messages, mode), 'visible')
  }
  assert.equal(actualLastMessageId([], [], 'conversation'), null)
})

test('reaction chips follow the retained copy and notification reveal stays available', () => {
  const notification = { id: 'reaction', accountId: 'a', messageId: '<reaction@example.org>', reaction: { emoji: '👍', targetMessageId: 'same@example.org', provider: 'gmail' } }
  for (const [first, winner] of [
    [{ id: 'all', folderId: 'all', bodyFetched: true }, { id: 'inbox', folderId: 'inbox', bodyFetched: true }],
    [{ id: 'empty', folderId: 'all', bodyFetched: false }, { id: 'fetched', folderId: 'all', bodyFetched: true }],
  ]) {
    const messages = [first, winner].map(message => ({ ...message, accountId: 'a', messageId: '<same@example.org>' })).concat(notification)
    const state = actualReaderState(messages, 'conversation')
    assert.deepEqual(state.readerMessages.map(message => message.id), [winner.id])
    assert.equal(state.reactions.groups.get(winner.id)?.[0].emoji, '👍')
    assert.equal(state.reactions.groups.get(winner.id)?.[0].messages[0].id, 'reaction')
    assert.equal(state.reactions.groups.has(first.id), false)
    const revealed = actualReaderState(messages, 'conversation', new Set(['reaction']))
    assert.deepEqual(revealed.readerMessages.map(message => message.id), [winner.id, 'reaction'])
    assert.deepEqual(actualReaderState(messages, 'traditional').readerMessages.map(message => message.id), [winner.id, 'reaction'])
  }
})

test('body fetch skips hidden duplicate copies even when given the full raw thread', async () => {
  const messages = [
    { id: 'hidden', accountId: 'a', messageId: '<same@example.org>', folderId: 'all', bodyFetched: false },
    { id: 'visible', accountId: 'a', messageId: '<same@example.org>', folderId: 'inbox', bodyFetched: false },
  ]
  const { readerMessages } = actualReaderState(messages, 'conversation')
  const handler = viewer.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === 'fetchUnfetchedBodies')
  const js = ts.transpileModule(handler.getText(viewer), { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
  const calls = []
  const fetch = new Function('readerMessages', 'conversation', 'FetchMessageBodyForAccount', 'const viewGeneration = 0; let bodyFetchErrors = {};\n' + js + '\nreturn fetchUnfetchedBodies')(
    readerMessages, { messages }, async (accountId, id) => { calls.push([accountId, id]); return null },
  )
  await fetch(messages)
  assert.deepEqual(calls, [['a', 'visible']])
})
