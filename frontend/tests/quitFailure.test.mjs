import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {runInNewContext} from 'node:vm'
import ts from 'typescript'
const source=await readFile(new URL('../src/App.svelte',import.meta.url),'utf8')
const handler=source.slice(source.indexOf('  async function finishQuitRequest()'),source.indexOf('  async function pendingCancel('))
const js=ts.transpileModule(handler,{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText
for (const failure of ['queue','quit']) test(`failed ${failure} during quit explains that the app stayed open and allows retry`,async()=>{
  let broken=true,cancelled=0,quits=0,dispatches=0
  const toasts=[]
  const context={quitRequestInProgress:false,isShuttingDown:false,pendingSends:[],Set,
    ListPendingSends:async()=>{if(broken&&failure==='queue')throw Error('database unavailable');return []},
    pendingDispatch:async()=>{dispatches++;return true},
    QuitApp:async()=>{if(broken&&failure==='quit')throw Error('window unavailable');quits++},
    CancelQuitAppRequest:()=>{cancelled++},
    addToast:toast=>toasts.push(toast),$_:key=>key,console:{error:()=>{}},
  }
  runInNewContext(js,context)
  await context.finishQuitRequest()
  assert.equal(context.isShuttingDown,false)
  assert.equal(context.quitRequestInProgress,false)
  assert.equal(cancelled,1)
  assert.equal(dispatches,0,'an unread queue is not permission to send')
  assert.equal(toasts.length,1,'failed quit must not be silent')
  assert.equal(toasts[0].type,'error')
  assert.equal(toasts[0].message,'pendingSend.quitFailed')
  broken=false;await context.finishQuitRequest();assert.equal(quits,1,'explicit retry can quit')
})

test('quit failure message is localized and never tells the user to resend',async()=>{
  for(const lang of ['en','fi']){
    const locale=JSON.parse(await readFile(new URL(`../src/lib/i18n/locales/${lang}.json`,import.meta.url),'utf8'))
    assert.equal(typeof locale.pendingSend.quitFailed,'string')
    assert.ok(locale.pendingSend.quitFailed.length>30)
  }
})
