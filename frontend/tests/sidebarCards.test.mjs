import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/sidebar/cards.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { normalizeCards, moveEntry, moveCard, quickEntries, shortcutIndex } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const entries = [{ id: 'inboxes', kind: 'inboxes', name: 'All Inboxes' }, { id: 'sent', kind: 'system', name: 'Sent', folderType: 'sent' }, { id: 'folder:a:f', kind: 'folder', accountId: 'a', folderId: 'f', name: 'Work', path: 'Work' }]
test('migration groups inboxes, system folders and pins with apps off', () => {
  const state = normalizeCards('', entries)
  assert.equal(state.showApps, false)
  assert.ok(state.cards.find(c => c.id === 'virtual').entries.includes('inboxes'))
  assert.ok(state.cards.find(c => c.id === 'pinned').entries.includes('folder:a:f'))
})
test('moving an entry between cards is immutable and unique', () => {
  const state = normalizeCards('', entries)
  const changed = moveEntry(state, 'sent', 'pinned', 0)
  assert.equal(changed.cards.find(c => c.id === 'pinned').entries[0], 'sent')
  assert.equal(changed.cards.flatMap(c => c.entries).filter(id => id === 'sent').length, 1)
  assert.ok(state.cards.find(c => c.id === 'primary').entries.includes('sent'))
})
test('saved custom groups retain order, hidden and collapsed state while adding new entries', () => {
  const state = normalizeCards(JSON.stringify({ cards: [{id:'custom', name:'Personal', hidden:true, collapsed:true, entries:['sent','sent','missing']}], quick:['sent','sent','missing'] }), entries)
  assert.deepEqual(state.cards[0], {id:'custom',name:'Personal',hidden:true,collapsed:true,entries:['sent']})
  assert.deepEqual(quickEntries(state, entries).map(e => e.id), ['sent'])
  assert.equal(moveCard(state, 'custom', 1).cards[1].id, 'custom')
})
test('number shortcuts use key position and ignore Shift/Alt', () => {
  assert.equal(shortcutIndex({code:'Digit1',key:'1',metaKey:true,ctrlKey:false,shiftKey:false,altKey:false}, true),0)
  assert.equal(shortcutIndex({code:'Digit0',key:'0',metaKey:false,ctrlKey:true,shiftKey:false,altKey:false}, false),9)
  assert.equal(shortcutIndex({code:'Digit1',key:'1',metaKey:true,shiftKey:true}, true),-1)
})
