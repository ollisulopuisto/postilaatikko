// Switching folders must not auto-select the top message: no row is
// highlighted and the reading pane keeps its empty state until the user
// picks a conversation. Keyboard navigation still starts sensibly from no
// selection (Down selects the first row). Returning to a folder visited
// this session still restores its remembered selection, and reader/history
// restore (back/forward, open from search) reapplies its own selection.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const list = await read('../src/lib/components/list/MessageList.svelte')

const loadBody = list.slice(list.indexOf('async function loadConversations'), list.indexOf('export async function syncFolder'))

test('loading a folder never force-selects a row', () => {
  assert.doesNotMatch(loadBody, /focusConversation\(conversations\[0\]\)/)
  assert.doesNotMatch(loadBody, /folderChanged \|\| !selectedThreadId/)
  // An empty folder still clears the selection explicitly.
  assert.match(loadBody, /focusConversation\(undefined\)/)
})

test('the virtual-folder paths do not auto-select either', () => {
  assert.doesNotMatch(list, /focusConversation\(conversations\[0\]\)/)
})

test('returning to a visited folder still restores its remembered selection', () => {
  assert.match(list, /remembered\.selectedThreadId &&\s*conversations\.some/)
  assert.match(list, /findFocusedRow\(conversations, remembered\.focusedRowIdentity/)
})

test('reader/history restore keeps reapplying its own selection after loads', () => {
  assert.match(list, /if \(readerRow\) focusConversation\(readerRow\)/)
})

test('keyboard navigation starts at the first row when nothing is selected', () => {
  // getSelectedIndex() reports -1 with no selection ...
  assert.match(list, /function getSelectedIndex\(\): number \{\s*if \(!selectedThreadId\) return -1/)
  const next = list.slice(list.indexOf('export function selectNext('), list.indexOf('export function selectFirst('))
  // ... and selectNext moves from -1 to index 0, the first row.
  assert.match(next, /const newIndex = currentIndex >= activeList\.length - 1 \? activeList\.length - 1 : currentIndex \+ 1/)
  const prev = list.slice(list.indexOf('export function selectPrevious('), list.indexOf('export function selectNext('))
  assert.match(prev, /const newIndex = currentIndex <= 0 \? 0 : currentIndex - 1/)
})

test('explicit selection still opens conversations in the reader', () => {
  // The reader only opens through the explicit select callback, so a plain
  // folder switch can no longer populate it.
  assert.match(list, /onConversationSelect\?\./)
})
