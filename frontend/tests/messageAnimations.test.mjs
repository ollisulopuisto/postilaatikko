import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function loadAnimationsStore() {
  let source = await readFile(new URL('../src/lib/stores/messageAnimations.svelte.ts', import.meta.url), 'utf8')
  source = source.replace(/^import .*$/gm, '').replace(/export /g, '')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  const factory = new Function(
    '$state',
    outputText +
      '\nreturn { messageAnimations: new MessageAnimationsStore(), MESSAGE_EXIT_ANIM_DURATION_MS }'
  )
  return factory(x => x)
}

test('messageAnimations store tracks single message removal and finishes properly', async () => {
  const { messageAnimations, MESSAGE_EXIT_ANIM_DURATION_MS } = await loadAnimationsStore()
  assert.equal(MESSAGE_EXIT_ANIM_DURATION_MS, 220)
  messageAnimations.clear()

  assert.equal(messageAnimations.isAnimating('msg-1'), undefined)
  assert.equal(messageAnimations.isAnyAnimating(), false)

  messageAnimations.startRemoval(['msg-1'], 'archive')
  assert.equal(messageAnimations.isAnimating('msg-1'), 'archive')
  assert.equal(messageAnimations.hasAnimating(['msg-1']), 'archive')
  assert.equal(messageAnimations.hasAnimating(['msg-2', 'msg-1']), 'archive')
  assert.equal(messageAnimations.isAnyAnimating(), true)

  messageAnimations.finishRemoval(['msg-1'])
  assert.equal(messageAnimations.isAnimating('msg-1'), undefined)
  assert.equal(messageAnimations.hasAnimating(['msg-1']), undefined)
  assert.equal(messageAnimations.isAnyAnimating(), false)
})

test('messageAnimations store supports distinct operations (trash, move, spam)', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.clear()

  messageAnimations.startRemoval(['msg-trash'], 'trash')
  messageAnimations.startRemoval(['msg-move'], 'move')
  messageAnimations.startRemoval(['msg-spam'], 'spam')

  assert.equal(messageAnimations.isAnimating('msg-trash'), 'trash')
  assert.equal(messageAnimations.isAnimating('msg-move'), 'move')
  assert.equal(messageAnimations.isAnimating('msg-spam'), 'spam')

  assert.equal(messageAnimations.hasAnimating(['unrelated', 'msg-trash']), 'trash')
  assert.equal(messageAnimations.hasAnimating(['msg-move']), 'move')

  messageAnimations.clear()
  assert.equal(messageAnimations.isAnyAnimating(), false)
  assert.equal(messageAnimations.isAnimating('msg-trash'), undefined)
})

test('messageAnimations store handles batch message IDs correctly', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.clear()

  const batch = ['id-1', 'id-2', 'id-3']
  messageAnimations.startRemoval(batch, 'archive')

  assert.equal(messageAnimations.isAnimating('id-1'), 'archive')
  assert.equal(messageAnimations.isAnimating('id-2'), 'archive')
  assert.equal(messageAnimations.isAnimating('id-3'), 'archive')

  messageAnimations.finishRemoval(['id-2'])
  assert.equal(messageAnimations.isAnimating('id-2'), undefined)
  assert.equal(messageAnimations.isAnimating('id-1'), 'archive')
  assert.equal(messageAnimations.isAnimating('id-3'), 'archive')

  messageAnimations.finishRemoval(['id-1', 'id-3'])
  assert.equal(messageAnimations.isAnyAnimating(), false)
})


test('reader-only operation settles without a mounted list and leaves other operations alone', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.startRemoval(['reader'], 'move')
  messageAnimations.startRemoval(['other'], 'archive')
  messageAnimations.settleRemoval(['reader'])
  await new Promise(resolve => setTimeout(resolve, 250))
  assert.equal(messageAnimations.isAnimating('reader'), undefined)
  assert.equal(messageAnimations.isAnimating('other'), 'archive')
})

test('slow replacement data holds removed rows hidden until reload finishes', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.startRemoval(['old-row'], 'trash')
  const release = messageAnimations.holdActive()
  messageAnimations.settleRemoval(['old-row'])
  await new Promise(resolve => setTimeout(resolve, 280))
  assert.equal(messageAnimations.isAnimating('old-row'), 'trash')
  release()
  assert.equal(messageAnimations.isAnimating('old-row'), undefined)
})

test('settlement from an older operation cannot clear a newer animation for the same message', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.startRemoval(['shared'], 'move')
  messageAnimations.settleRemoval(['shared'])
  messageAnimations.startRemoval(['shared'], 'spam')
  await new Promise(resolve => setTimeout(resolve, 250))
  assert.equal(messageAnimations.isAnimating('shared'), 'spam')
})

test('Trash quick-delete waits for confirmation without hiding its row', async () => {
  const row = await readFile(new URL('../src/lib/components/list/ConversationRow.svelte', import.meta.url), 'utf8')
  assert.match(row, /if \(folderType !== 'trash'\) messageAnimations.startRemoval/)
  const list = await readFile(new URL('../src/lib/components/list/MessageList.svelte', import.meta.url), 'utf8')
  assert.doesNotMatch(list, /messageAnimations.clear\(\)/)
  assert.match(list, /doHandleActionComplete\(autoSelectNext\).finally\(release\)/)
})


test('completed reload holds discard all processed ID bookkeeping', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.startRemoval(['a', 'b'], 'archive')
  const release = messageAnimations.holdActive()
  release()
  assert.equal(messageAnimations.holds.size, 0)
  assert.equal(messageAnimations.generations.size, 0)
})


async function loadArchiveAndMarkReadHandler(dependencies) {
  const source = await readFile(new URL('../src/lib/components/viewer/ConversationViewer.svelte', import.meta.url), 'utf8')
  const script = source.slice(source.indexOf('>', source.indexOf('<script')) + 1, source.indexOf('</script>'))
  const parsed = ts.createSourceFile('ConversationViewer.ts', script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const handler = parsed.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === 'handleArchiveAndMarkRead')
  assert.ok(handler, 'the real archive-and-read handler must be exercised')
  const { outputText } = ts.transpileModule(handler.getText(parsed), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return new Function(...Object.keys(dependencies), 'let moveMutationInFlight = false;\n' + outputText + '\nreturn handleArchiveAndMarkRead')(...Object.values(dependencies))
}

test('archive-and-read animates and mutates only resolved matched targets', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  const calls = []
  let resolveTargets
  const targets = new Promise(resolve => { resolveTargets = resolve })
  const handler = await loadArchiveAndMarkReadHandler({
    conversation: { messages: [{ id: 'matched' }, { id: 'unmatched' }] },
    resolveDisplayedMessageTarget: ids => { calls.push(['resolve', ids]); return targets },
    messageAnimations,
    runMessageMutation: async (ids, mutation) => {
      calls.push(['mutate', ids])
      assert.equal(messageAnimations.isAnimating('matched'), 'archive')
      assert.equal(messageAnimations.isAnimating('unmatched'), undefined)
      await mutation()
    },
    ArchiveWithUndo: async ids => { calls.push(['archive', ids]); return { operationId: 'op' } },
    ArchiveAndMarkReadWithUndo: async ids => { calls.push(['archiveRead', ids]); return { operationId: 'op', atomic: true } },
    MarkAsRead: async ids => { calls.push(['read', ids]) },
    publishUndoOperationCreated: () => {},
    rememberUnreadForUndo: () => {},
    showArchiveUndoToast: () => {},
    onActionComplete: () => {},
    toasts: { error: message => assert.fail(message) },
    $_: key => key,
    console,
  })
  const pending = handler()
  assert.equal(messageAnimations.isAnyAnimating(), false, 'no removal before target resolution')
  resolveTargets(['matched'])
  await pending
  assert.deepEqual(calls, [
    ['resolve', ['matched', 'unmatched']],
    ['mutate', ['matched']],
    ['archiveRead', ['matched']],
  ])
  await new Promise(resolve => setTimeout(resolve, 250))
  assert.equal(messageAnimations.isAnyAnimating(), false)
  assert.equal(messageAnimations.isAnimating('unmatched'), undefined)
})
