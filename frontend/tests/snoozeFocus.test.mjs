import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/components/list/focusedRow.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const { focusedRowKey, findFocusedRow } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)
const rows = [
  { accountId: 'A', folderId: 'inbox-A', threadId: 'shared' },
  { accountId: 'B', folderId: 'inbox-B', threadId: 'shared' },
]

test('auto-selecting the first unified row works without any reader selection', () => {
  const key = focusedRowKey(rows[0], 'unified', 'inbox')
  assert.equal(findFocusedRow(rows, key, 'shared', 'unified', 'inbox'), rows[0])
})
test('list-only keyboard focus on the second duplicate keeps its account and folder', () => {
  const key = focusedRowKey(rows[1], 'unified', 'inbox')
  assert.equal(findFocusedRow(rows, key, 'shared', 'unified', 'inbox'), rows[1])
  // Reordering the rows does not switch the target back to the first account.
  assert.equal(findFocusedRow([...rows].reverse(), key, 'shared', 'unified', 'inbox'), rows[1])
})
test('stale identity and ambiguous thread-only selection never snooze another account', () => {
  const key = focusedRowKey(rows[1])
  assert.equal(findFocusedRow([rows[0]], key, 'shared'), undefined)
  assert.equal(findFocusedRow(rows, null, 'shared'), undefined)
})
test('same-account rows in different folders retain mailbox-qualified identity', () => {
  const copies = [rows[0], { ...rows[0], folderId: 'archive-A' }]
  assert.equal(findFocusedRow(copies, focusedRowKey(copies[1]), 'shared'), copies[1])
})
test('folder-local rows may use the account and folder supplied by the list', () => {
  const local = [{ threadId: 'local-thread' }]
  const key = focusedRowKey(local[0], 'A', 'inbox-A')
  assert.equal(findFocusedRow(local, key, 'local-thread', 'A', 'inbox-A'), local[0])
})

const listSource = await readFile(new URL('../src/lib/components/list/MessageList.svelte', import.meta.url), 'utf8')
const setter = listSource.slice(listSource.indexOf('  function focusConversation('), listSource.indexOf('  // Reader/history restoration'))
const snooze = listSource.slice(listSource.indexOf('  export function snoozeSelection'), listSource.indexOf('  export function toggleMoveToDialog'))
const harnessCode = ts.transpileModule(source.replaceAll('export ', '') + setter + snooze.replace('export ', ''), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const makeList = new Function('conversations', `
  const accountId='unified',folderId='inbox',checkedThreadIds=new Set(),selectedQueueReady=true,selectedQueueRefs=[];
  const isSearchMode=false,serverSearchMode=false,serverSearchResults=[],searchResults=[],readerSelection=null;
  let selectedThreadId=null,focusedRowIdentity=null; const shown=[];
  const snoozeDialog={show:targets=>{shown.push(targets);return true}};
  ${harnessCode}
  return {focus:focusConversation,snooze:snoozeSelection,shown};
`)

test('the actual snooze command opens for an auto-selected duplicate row without an opened reader', () => {
  const list = makeList(rows)
  list.focus(rows[0])
  list.snooze()
  assert.deepEqual(list.shown, [[{ accountId: 'A', conversationId: 'shared' }]])
})
test('the actual snooze command follows list focus to the second duplicate, not the old reader account', () => {
  const list = makeList(rows)
  list.focus(rows[0])
  list.focus(rows[1])
  list.snooze()
  assert.deepEqual(list.shown, [[{ accountId: 'B', conversationId: 'shared' }]])
})
