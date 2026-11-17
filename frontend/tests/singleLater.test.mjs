import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import ts from 'typescript'
function makeQueue(read=[],reply=[],failReply=false,noReceipt=false){
  const calls=[],undos=[]
  const source=readFileSync(new URL('../src/lib/stores/later.svelte.ts',import.meta.url),'utf8').replace(/^import .*$/mg,'')
  const code=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText
  const r={refs:read,ready:true,load:async()=>{},has:(a,c)=>r.refs.some(x=>x.accountId===a&&x.conversationId===c),isBusy:()=>false,setWithUndo:async(a,c,on)=>{calls.push(['read',a,c,on]);r.refs=on?[...r.refs,{accountId:a,conversationId:c}]:r.refs.filter(x=>x.accountId!==a||x.conversationId!==c);return{changed:true,operationId:noReceipt?'':'read-op'}}}
  const q={refs:reply,ready:true,load:async()=>{},has:(a,c)=>q.refs.some(x=>x.accountId===a&&x.conversationId===c),isBusy:()=>false,set:async(a,c,on)=>{calls.push(['reply',a,c,on]);if(failReply)throw Error('offline');q.refs=q.refs.filter(x=>x.accountId!==a||x.conversationId!==c);return{changed:true,operationId:'reply-op'}}}
  const exports={};new Function('exports','readLaterQueue','replyLaterQueue','$state','UndoOperation',code)(exports,r,q,x=>x,async id=>undos.push(id));return{...exports,calls,undos}
}
const ref=(a,c)=>({accountId:a,conversationId:c})
test('one pile deduplicates both old queues and keeps account identity',()=>{const {laterQueue:q}=makeQueue([ref('a','1')],[ref('a','1'),ref('b','1')]);assert.equal(q.count,2);assert(q.has('b','1'));assert(!q.has('c','1'))})
test('new marks use existing storage and duplicate adds are no-ops',async()=>{const x=makeQueue();await x.laterQueue.setWithUndo('a','1',true);assert.deepEqual(x.calls,[['read','a','1',true]]);await x.laterQueue.setWithUndo('a','1',true);assert.equal(x.calls.length,1)})
test('removal clears both legacy flags and targeted undo restores both receipts',async()=>{const x=makeQueue([ref('a','1')],[ref('a','1')]);const result=await x.laterQueue.setWithUndo('a','1',false);assert.equal(x.calls.length,2);await x.undoLaterChange(result);assert.deepEqual(x.undos,['reply-op','read-op']);assert(!x.laterQueue.has('a','1'))})
test('partial removal attempts rollback and never reports success',async()=>{const x=makeQueue([ref('a','1')],[ref('a','1')],true);await assert.rejects(x.laterQueue.setWithUndo('a','1',false),/offline/);assert.deepEqual(x.undos,['read-op'])})

test('standalone removal without receipt restores read mark if the other legacy write fails',async()=>{const x=makeQueue([ref('a','1')],[ref('a','1')],true,true);await assert.rejects(x.laterQueue.setWithUndo('a','1',false),/offline/);assert.deepEqual(x.calls.filter(c=>c[0]==='read').map(c=>c[3]),[false,true]);assert(x.laterQueue.has('a','1'))})
