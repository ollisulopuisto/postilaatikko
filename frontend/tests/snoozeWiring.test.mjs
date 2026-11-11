import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const read = path => readFile(new URL(path, import.meta.url), 'utf8')

test('a detached mail window hosts its own Snooze dialog, the embedded one reuses the primary window dialog', async () => {
  const source = await read('../src/MailWindowApp.svelte')
  assert.match(source, /\{#if !embedded\}<SnoozeDialog bind:open=\{snoozeDialog\.open\}/)
  assert.match(source, /onDone=\{\(\) => list\?\.handleActionComplete\(true\)\}/)
})

test('snoozeSelection owns checked rows and the focused row, with the queue canonical guard', async () => {
  const source = await read('../src/lib/components/list/MessageList.svelte')
  const body = source.slice(source.indexOf('export function snoozeSelection'), source.indexOf('export function toggleMoveToDialog'))
  assert.match(body, /checkedThreadIds\.size > 0/)
  assert.match(body, /if \(selectedQueueReady\) snoozeDialog\.show\(selectedQueueRefs\)/)
  assert.match(body, /_queueCanonical === false\) return true/)
  assert.match(body, /_canonicalConversationId \|\| focused\.threadId/)
})

test('snooze uses an account-qualified row key, and the reader only owns the viewer pane', async () => {
  const source = await read('../src/lib/components/list/MessageList.svelte')
  const body = source.slice(source.indexOf('export function snoozeSelection'), source.indexOf('export function toggleMoveToDialog'))
  assert.match(body, /findFocusedRow\(activeRows, focusedRowIdentity, selectedThreadId/)
  assert.match(source, /focusConversation\(activeList\[0\]\)/)
  const app = await read('../src/App.svelte')
  const command = app.split("registerCommand('mail.snooze',")[1].split('registerCommand(')[0]
  assert.match(command, /getFocusedPane\(\) === 'viewer'/)
  assert.doesNotMatch(command, /if \(!messageListRef/)
})
