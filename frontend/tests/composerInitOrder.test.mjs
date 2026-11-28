import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const composer = await readFile(new URL('../src/lib/components/composer/Composer.svelte', import.meta.url), 'utf8')
const onMount = composer.slice(composer.indexOf('onMount(async () => {'))

test('the editor is created before the first awaited backend call in onMount', () => {
  const editorAt = onMount.indexOf('ensureEditor()')
  const firstAwait = onMount.indexOf('await ')
  assert.ok(editorAt > -1 && firstAwait > -1)
  assert.ok(editorAt < firstAwait, 'ensureEditor() must run before any await so a stalled call cannot leave the body uneditable')
})

test('a failed identity load is shown to the user, not only logged', () => {
  const failure = onMount.indexOf("console.error('Failed to load identities:'")
  assert.ok(failure > -1)
  assert.match(onMount.slice(failure, failure + 400), /addToast\(\{ type: 'error'/)
})
