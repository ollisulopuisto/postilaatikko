import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
const row=readFileSync(new URL('../src/lib/components/list/ConversationRow.svelte',import.meta.url),'utf8')
const list=readFileSync(new URL('../src/lib/components/list/MessageList.svelte',import.meta.url),'utf8')
test('all row render paths receive the checked-set state even while message IDs are unresolved',()=>{
  const rows=[...list.matchAll(/<ConversationRow\b[\s\S]*?\/>/g)]
  assert.equal(rows.length,4)
  for(const [tag]of rows)assert.match(tag,/selectionActive=\{checkedThreadIds.size > 0\}/)
})
test('O6 unmounts row action buttons during selection rather than hiding tabbable controls',()=>{
  assert.match(row,/selectionActive\?: boolean/)
  assert.match(row,/\{#if !selectionActive\}\s*<!-- Quick actions/)
  assert.match(row,/class="quick-row-action[\s\S]*?\{\/if\}\s*<!-- Checkbox column/)
})

test('selection object geometry excludes unchecked gaps and handles changing row heights', async()=>{
  const ts=(await import('typescript')).default
  const source=readFileSync(new URL('../src/lib/components/list/selectionGeometry.ts',import.meta.url),'utf8')
  const {outputText}=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}})
  const {selectionRuns}=await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
  const row=(top,bottom,checked)=>({top,bottom,checked})
  assert.deepEqual(selectionRuns([]),[])
  assert.deepEqual(selectionRuns([row(0,40,true),row(40,90,true),row(90,130,false),row(130,210,true)]),[{top:0,bottom:90},{top:130,bottom:210}])
  assert.deepEqual(selectionRuns([row(-50,30,true),row(30,100,false),row(100,180,true)]),[{top:-50,bottom:30},{top:100,bottom:180}])
  assert.deepEqual(selectionRuns([row(0,40,false)]),[])
  assert.deepEqual(selectionRuns([row(0,40,true),row(70,110,true)]),[{top:0,bottom:40},{top:70,bottom:110}])
})
