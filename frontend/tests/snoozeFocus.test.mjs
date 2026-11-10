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

const script = listSource.slice(listSource.indexOf('>', listSource.lastIndexOf('<script', listSource.indexOf('function focusConversation'))) + 1, listSource.indexOf('</script>', listSource.indexOf('function focusConversation')))
const parsed = ts.createSourceFile('list.ts', script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
const actions = ['getSelectedThreadId', 'getSelectedMessageIds', 'getSelectedMessageRefs', 'getSelectedConversationInfo', 'isSelectedStarred', 'isSelectedUnread', 'getSelectedIndex', 'selectNext']
const methods = parsed.statements.filter(n => ts.isFunctionDeclaration(n) && actions.includes(n.name?.text)).map(n => n.getText(parsed).replace(/^export /, '')).join('\n')
const actionCode = ts.transpileModule(source.replaceAll('export ', '') + setter + methods, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const makeActionList = new Function('activeList', `
  const accountId='unified',folderId='inbox',isUnifiedView=true,isSearchMode=false,activeCount=activeList.length;
  const document={activeElement:null},scrollToIndex=()=>{}; let loadMoreButtonRef=null;
  const refsForConversation=c=>c.messageIds.map(messageId=>({accountId:c.accountId,messageId}));
  let selectedThreadId=null,focusedRowIdentity=null;
  ${actionCode}
  return {focus:focusConversation,${actions.join(',')}};
`)
const actionRows = [
  { ...rows[0], messageIds: ['a-message'], _canonicalConversationId:'a-canonical', isStarred:false, unreadCount:0 },
  { ...rows[1], messageIds: ['b-message'], _canonicalConversationId:'b-canonical', isStarred:true, unreadCount:2 },
]

test('keyboard j followed by an action targets the highlighted second account, with matching state', () => {
  const list = makeActionList(actionRows)
  list.focus(actionRows[0]); list.selectNext()
  assert.equal(list.getSelectedIndex(),1)
  // App's archive/read/trash context takes these refs, not a bare thread ID.
  assert.deepEqual(list.getSelectedMessageRefs(), [{accountId:'B',messageId:'b-message'}])
  assert.deepEqual(list.getSelectedMessageIds(), ['b-message'])
  assert.equal(list.getSelectedThreadId(),'b-canonical')
  assert.deepEqual(list.getSelectedConversationInfo(),{accountId:'B',folderId:'inbox-B'})
  assert.equal(list.isSelectedStarred(),true)
  assert.equal(list.isSelectedUnread(),true)
})

test('stale focused account never falls back to the other account for a mailbox mutation', () => {
  const remaining = [...actionRows]
  const list = makeActionList(remaining)
  list.focus(actionRows[1]); remaining.pop()
  assert.deepEqual(list.getSelectedMessageRefs(),[])
  assert.deepEqual(list.getSelectedMessageIds(),[])
  assert.equal(list.getSelectedThreadId(),null)
  assert.equal(list.getSelectedConversationInfo(),null)
  assert.equal(list.isSelectedStarred(),false)
  assert.equal(list.isSelectedUnread(),false)
})
