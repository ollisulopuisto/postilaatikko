import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'
const path=new URL('../src/lib/components/command/paletteFolders.ts',import.meta.url)
function load(){
  const source=readFileSync(path,'utf8')
  const js=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText
  const exports={};vm.runInNewContext(js,{exports});return exports
}
const folder=(id,name,path=name,extra={})=>({id,name,path,type:'folder',...extra})
const accounts=[{account:{id:'a',name:'Work',email:'work@example.test'},folders:[]},{account:{id:'b',name:'Home',email:'home@example.test'},folders:[]}]
const catalog={a:[folder('nested','Past Invoices','Clients/Past Invoices'),folder('exact','Invoice'),folder('prefix','Invoice archive'),folder('hidden','Invoice hidden','Invoice hidden',{hidden:true})],b:[folder('same','Invoice')]}
test('folder query ranks exact before prefix before substring across accounts',()=>{
  const {matchPaletteFolders}=load();const rows=matchPaletteFolders(accounts,catalog,'invoice')
  assert.deepEqual(Array.from(rows,r=>r.folderId),['exact','same','prefix','nested'])
  assert.equal(rows[0].accountId,'a');assert.equal(rows[1].accountId,'b');assert.ok(rows[3].context.includes('Clients'))
})
test('folder matches are case insensitive, bounded and absent on blank query',()=>{
  const {matchPaletteFolders}=load();assert.equal(matchPaletteFolders(accounts,catalog,'  ').length,0)
  assert.equal(matchPaletteFolders(accounts,catalog,'INVOICE').length,4)
  assert.equal(matchPaletteFolders(accounts,{a:Array.from({length:100},(_,i)=>folder(String(i),'Folder '+i))},'folder').length,20)
})
test('tree fallback retains hierarchy, excludes hidden ancestry and deduplicates account-folder identity',()=>{
  const {matchPaletteFolders}=load()
  const tree=[{account:{id:'a',name:'Work'},folders:[{folder:folder('parent','Clients'),children:[{folder:folder('x','Invoices','Clients/Invoices')}]},{folder:folder('hidden','Secret','Secret',{hidden:true}),children:[{folder:folder('y','Invoices') }]}]}]
  assert.deepEqual(Array.from(matchPaletteFolders(tree,{},'invoice'),r=>r.folderId),['x'])
})
test('palette keeps commands ahead of folders and search last, with existing App navigation',()=>{
  const palette=readFileSync(new URL('../src/lib/components/command/CommandPalette.svelte',import.meta.url),'utf8')
  assert.match(palette,/\[\.\.\.filteredCommands, \.\.\.folderItems, \.\.\.\(query.trim\(\) \? \[searchFallbackItem\(\)\]/)
  const app=readFileSync(new URL('../src/App.svelte',import.meta.url),'utf8')
  const callback=app.match(/<CommandPalette onFolderSelect=\{(\(folder\) => \{[\s\S]*?)\}\} \/>/)
  assert.ok(callback,'palette has a folder handler')
  const calls=[]
  const run=vm.runInNewContext(ts.transpileModule('('+callback[1]+'})',{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText,{
    setActiveExtension:id=>calls.push(['view',id]),accountStore:{selectFolder:(...args)=>calls.push(['store',...args])},handleFolderSelect:(...args)=>calls.push(['navigate',...args]),
  })
  run({accountId:'b',folderId:'same',path:'Family/Invoices',name:'Invoices',type:'folder'})
  assert.deepEqual(calls,[['view','mail'],['store','b','same','Family/Invoices','Invoices'],['navigate','b','same','Family/Invoices','Invoices','folder']])
})
