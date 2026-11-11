// Switching folders must not auto-select the top message: no row is
// highlighted and the reading pane keeps its empty state until the user
// picks a conversation. Keyboard navigation still starts sensibly from no
// selection (Down selects the first row). Returning to a folder visited
// this session still restores its remembered selection, and reader/history
// restore (back/forward, open from search) reapplies its own selection.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
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
  // Both restore spots route through the tested navigation policy.
  const restores = list.match(/selectionAfterFolderSwitch\(conversations, remembered/g) ?? []
  assert.equal(restores.length, 2, 'immediate and post-load restores both use the policy')
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

// --- Folder navigation and refresh selection policy, exercised as real
// code (the component imports the same module; wiring asserted below). ---

async function loadPolicy() {
  const readTs = p => readFile(new URL(p, import.meta.url), 'utf8')
  const compile = source => ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const focused = compile((await readTs('../src/lib/components/list/focusedRow.ts')).replaceAll('export ', ''))
  const reconcile = compile((await readTs('../src/lib/components/list/selectionReconcile.ts'))
    .replace(/import \{[^}]*\} from '\.\/focusedRow'/, '')
    .replaceAll('export ', ''))
  return new Function(`${focused}\n${reconcile}\n;return { selectionAfterFolderSwitch, selectionSurvivesRefresh }`)()
}

const { selectionAfterFolderSwitch, selectionSurvivesRefresh } = await loadPolicy()

const row = (threadId, accountId, folderId) => ({ threadId, accountId, folderId })
const keyOf = (r, accountId = '', folderId = '') => JSON.stringify([r.accountId || accountId, r.folderId || folderId, r.threadId])

test('switching to an unvisited folder selects nothing, so the reader stays empty', () => {
  const rowsB = [row('x1', 'a1', 'B'), row('x2', 'a1', 'B')]
  assert.equal(selectionAfterFolderSwitch(rowsB, undefined, 'a1', 'B'), undefined)
  assert.equal(selectionAfterFolderSwitch(rowsB, { selectedThreadId: null, focusedRowIdentity: null }, 'a1', 'B'), undefined)
})

test('returning to a visited folder restores its remembered row', () => {
  const rowsA = [row('t1', 'a1', 'A'), row('t2', 'a1', 'A')]
  const remembered = { selectedThreadId: 't2', focusedRowIdentity: keyOf(rowsA[1], 'a1', 'A') }
  assert.equal(selectionAfterFolderSwitch(rowsA, remembered, 'a1', 'A'), rowsA[1])
})

test('a remembered row that vanished while away does not resurrect a selection', () => {
  const rowsA = [row('t1', 'a1', 'A')]
  const remembered = { selectedThreadId: 't2', focusedRowIdentity: JSON.stringify(['a1', 'A', 't2']) }
  assert.equal(selectionAfterFolderSwitch(rowsA, remembered, 'a1', 'A'), undefined)
})

test('in unified folders the remembered account wins over a same-threadId row', () => {
  const rows = [row('t1', 'a1', 'U'), row('t1', 'a2', 'U')]
  const remembered = { selectedThreadId: 't1', focusedRowIdentity: JSON.stringify(['a2', 'U', 't1']) }
  assert.equal(selectionAfterFolderSwitch(rows, remembered, '', 'U')?.accountId, 'a2')
})

test('same-folder refresh keeps a selection whose row is still present', () => {
  const rows = [row('t1', 'a1', 'F'), row('t2', 'a1', 'F')]
  assert.equal(selectionSurvivesRefresh(rows, keyOf(rows[1], 'a1', 'F'), 't2', 'a1', 'F'), true)
})

test('same-folder refresh drops the selection when its account-qualified row is gone', () => {
  assert.equal(selectionSurvivesRefresh([row('t1', 'a1', 'F')], JSON.stringify(['a1', 'F', 't2']), 't2', 'a1', 'F'), false)
  // A same-threadId row from another account is not the selected row.
  assert.equal(selectionSurvivesRefresh([row('t2', 'a2', 'F')], JSON.stringify(['a1', 'F', 't2']), 't2', 'a1', 'F'), false)
})

test('no selection stays no selection across refreshes', () => {
  assert.equal(selectionSurvivesRefresh([row('t1', 'a1', 'F')], null, null, 'a1', 'F'), false)
})

// --- Component wiring: MessageList calls the policy at the right spots. ---

test('same-folder refreshes reconcile the selection against the returned rows', () => {
  const replace = list.slice(list.indexOf('async function replaceConversationRows'), list.indexOf('function showNewestMail'))
  assert.match(replace, /refresh && lastLoadedFolderId === folderId/)
  assert.match(replace, /selectionSurvivesRefresh\(conversations, focusedRowIdentity, selectedThreadId/)
  assert.match(replace, /focusConversation\(undefined\)/)
})

test('a remembered selection that fails reconciliation is cleared, never left phantom', () => {
  const effect = list.slice(list.lastIndexOf('loadConversations(restoreCount)'))
  assert.match(effect, /else if \(remembered\.selectedThreadId\) focusConversation\(undefined\)/)
})
