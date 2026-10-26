// P-036: search the folder checklist in the virtual folder dialog.
//
// filterFolders() keeps the folders whose label contains every word typed in
// the search field, ignoring case and extra spaces. The checklist keeps its
// selection while filtered, so this only decides what is shown.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function load(relative) {
  const source = await readFile(new URL(relative, import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}

const { filterFolders } = await load('../src/lib/sidebar/folderSearch.ts')

const folders = [
  { id: '1', label: 'GMail · _K-S/ranta' },
  { id: '2', label: 'GMail · _K-S/riku' },
  { id: '3', label: 'GMail · Work/Projects' },
  { id: '4', label: 'Fastmail · Archive/2024' },
  { id: '5', label: 'Fastmail · Lähetetyt' },
]

test('an empty or blank query keeps every folder', () => {
  assert.deepEqual(filterFolders(folders, ''), folders)
  assert.deepEqual(filterFolders(folders, '   '), folders)
})

test('a query matches folders whose label contains it, ignoring case', () => {
  assert.deepEqual(filterFolders(folders, 'RANTA').map(f => f.id), ['1'])
  assert.deepEqual(filterFolders(folders, 'riku').map(f => f.id), ['2'])
})

test('every word must match, in any position of the label', () => {
  assert.deepEqual(filterFolders(folders, 'gmail work').map(f => f.id), ['3'])
  assert.deepEqual(filterFolders(folders, 'k-s   riku').map(f => f.id), ['2'])
  assert.deepEqual(filterFolders(folders, 'gmail archive'), [])
})

test('the account name is searchable as well as the folder path', () => {
  assert.deepEqual(filterFolders(folders, 'fastmail').map(f => f.id), ['4', '5'])
})

test('Finnish letters match whether composed or decomposed', () => {
  const composed = 'Lähetetyt'
  const decomposed = 'Lähetetyt'
  assert.deepEqual(filterFolders(folders, decomposed).map(f => f.id), ['5'])
  assert.deepEqual(filterFolders(folders, composed.toLowerCase()).map(f => f.id), ['5'])
})

test('filterFolders does not mutate its input and returns no match for an unknown word', () => {
  const snapshot = JSON.stringify(folders)
  filterFolders(folders, 'ranta')
  assert.equal(JSON.stringify(folders), snapshot)
  assert.deepEqual(filterFolders(folders, 'nosuchfolder'), [])
})
