import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const navigationCards = await readFile(new URL('../src/lib/components/sidebar/NavigationCards.svelte', import.meta.url), 'utf8')
const folderTreeItem = await readFile(new URL('../src/lib/components/sidebar/FolderTreeItem.svelte', import.meta.url), 'utf8')
const sidebar = await readFile(new URL('../src/lib/components/sidebar/Sidebar.svelte', import.meta.url), 'utf8')
const replyLaterButton = await readFile(new URL('../src/lib/components/common/ReplyLaterButton.svelte', import.meta.url), 'utf8')

test('NavigationCards favorite button includes focus-visible ring styles and opacity disclosure', () => {
  const match = navigationCards.match(/<button[^>]*aria-label=\{\$_?\('navigation\.favorite'\)\}[^>]*>/)
  assert.ok(match, 'NavigationCards favorite button exists')
  const tag = match[0]
  assert.match(tag, /focus-visible:opacity-100/, 'reveals on focus-visible')
  assert.match(tag, /focus-visible:outline-none/, 'clears native outline on focus-visible')
  assert.match(tag, /focus-visible:ring-2/, 'has ring-2 on focus-visible')
  assert.match(tag, /focus-visible:ring-ring/, 'uses ring-ring color token')
})

test('FolderTreeItem favorite button includes focus-visible ring styles and opacity disclosure', () => {
  const match = folderTreeItem.match(/<button[^>]*aria-label=\{\$_?\('navigation\.favorite'\)\}[^>]*>/)
  assert.ok(match, 'FolderTreeItem favorite button exists')
  const tag = match[0]
  assert.match(tag, /focus-visible:opacity-100/, 'reveals on focus-visible')
  assert.match(tag, /focus-visible:outline-none/, 'clears native outline on focus-visible')
  assert.match(tag, /focus-visible:ring-2/, 'has ring-2 on focus-visible')
  assert.match(tag, /focus-visible:ring-ring/, 'uses ring-ring color token')
})

test('Sidebar all-folders favorite button includes focus-visible ring styles and opacity disclosure', () => {
  const match = sidebar.match(/<button[^>]*aria-label=\{\$_?\('navigation\.favorite'\)\}[^>]*>/)
  assert.ok(match, 'Sidebar all-folders favorite button exists')
  const tag = match[0]
  assert.match(tag, /focus-visible:opacity-100/, 'reveals on focus-visible')
  assert.match(tag, /focus-visible:outline-none/, 'clears native outline on focus-visible')
  assert.match(tag, /focus-visible:ring-2/, 'has ring-2 on focus-visible')
  assert.match(tag, /focus-visible:ring-ring/, 'uses ring-ring color token')
})

test('ReplyLaterButton includes focus-visible ring styles and opacity disclosure', () => {
  const match = replyLaterButton.match(/<button[^>]*title=\{label\}[^>]*>/)
  assert.ok(match, 'ReplyLaterButton button tag exists')
  const tag = match[0]
  assert.match(tag, /focus-visible:opacity-100/, 'reveals on focus-visible')
  assert.match(tag, /focus-visible:outline-none/, 'clears native outline on focus-visible')
  assert.match(tag, /focus-visible:ring-2/, 'has ring-2 on focus-visible')
  assert.match(tag, /focus-visible:ring-ring/, 'uses ring-ring color token')
})
