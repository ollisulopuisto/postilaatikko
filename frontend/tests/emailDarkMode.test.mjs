import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import ts from 'typescript'
const source=readFileSync(new URL('../src/lib/utils/emailDarkMode.ts',import.meta.url),'utf8')
const js=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText
const module={exports:{}};new Function('exports','module',js)(module.exports,module)
const {adaptEmailDarkMode}=module.exports

test('authored dark media rules follow selected app mode, images never filtered',()=>{
 const media={conditionText:'(prefers-color-scheme: dark)',media:{mediaText:''}}
 const doc={querySelectorAll:()=>[{textContent:'@media (prefers-color-scheme: dark){body{color:white}}'}],documentElement:{style:{}},styleSheets:[{cssRules:[media]}]}
 assert.equal(adaptEmailDarkMode(doc),'authored')
 assert.equal(media.media.mediaText,'(min-width: 0px)')
 assert.equal(doc.documentElement.style.filter,undefined)
})

test('fallback changes painted light surfaces and dark text but not image data',()=>{
 const writes=[]
 const node=(tag,color,bg,image='none')=>({tagName:tag,closest:()=>false,style:{setProperty:(...args)=>writes.push([tag,...args])},computed:{color,backgroundColor:bg,backgroundImage:image}})
 const text=node('P','rgb(0, 0, 0)','rgb(255, 255, 255)')
 const link=node('A','rgb(0, 20, 80)','rgba(0, 0, 0, 0)')
 const art=node('DIV','rgb(0, 0, 0)','rgb(255, 255, 255)','url(image)')
 const style={setProperty:()=>{}}
 const doc={querySelectorAll:selector=>selector==='style'?[]:[text,link,art],defaultView:{getComputedStyle:n=>n.computed},documentElement:{style},body:{style}}
 assert.equal(adaptEmailDarkMode(doc),'adapted')
 assert.ok(writes.some(row=>row[0]==='P'&&row[1]==='background-color'))
 assert.ok(writes.some(row=>row[0]==='A'&&row[2]==='#90bcff'))
 assert.ok(writes.every(row=>row[0]!=='DIV'))
 assert.ok(writes.every(row=>row[1]!=='filter'))
})
