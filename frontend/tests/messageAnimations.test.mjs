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

test('messageAnimations auto-expires animating status after specified ttl', async () => {
  const { messageAnimations } = await loadAnimationsStore()
  messageAnimations.clear()

  messageAnimations.startRemoval(['id-ttl'], 'archive', 50)
  assert.equal(messageAnimations.isAnimating('id-ttl'), 'archive')

  await new Promise(resolve => setTimeout(resolve, 80))
  assert.equal(messageAnimations.isAnimating('id-ttl'), undefined)
  assert.equal(messageAnimations.isAnyAnimating(), false)
})
