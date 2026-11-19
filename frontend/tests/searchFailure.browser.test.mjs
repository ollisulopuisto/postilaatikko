import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync} from 'node:fs'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captureDir=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-search-failure-captures'
mkdirSync(captureDir,{recursive:true})
// Regression: a DMG built after MailSearchRequest moved behind a Go type alias
// shipped bindings without the constructor, so every search call threw
// TypeError at the callsite. The failure must surface as an error and leave
// the UI responsive — no stuck spinner, no retry pile-up, no frozen list.
test('a throwing search call shows an error and never freezes the search UI',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({root,plugins:[{name:'search-failure-fixture',resolveId(id){if(id==='/search-mount.js')return id},load(id){if(id==='/search-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5194,strictPort:true},logLevel:'error'})
  await server.listen()
  const {chromium}=await import(browserModule)
  const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1280,height:900}})
    const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/search-failure-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}))
    await page.goto('http://127.0.0.1:5194/search-failure-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css')
      window.calls=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(target,key)=>target[key]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>(..._args)=>{
        window.calls.push({name,at:performance.now()})
        if(name==='SearchMail')throw new TypeError("undefined is not a constructor (evaluating 'new MailSearchRequest()')")
        if(name==='GetFTSIndexStatus')return Promise.resolve({complete:true,totalMessages:0,indexedMessages:0})
        if(name==='IsFTSIndexing')return Promise.resolve(false)
        if(/Count/.test(name))return Promise.resolve(0)
        return Promise.resolve([])
      }})}}
      await (await import('/src/lib/i18n/index.ts')).initI18n('en')
      const {accountStore}=await import('/src/lib/stores/accounts.svelte.ts')
      accountStore.accounts=['a','b'].map(id=>({account:{id,name:`Account ${id}`,email:`${id}@example.test`,enabled:true,authType:'oauth2',imapHost:'imap.gmail.com'},folders:[],loading:false,syncing:false,error:null,lastSync:null}))
      const {mount}=await import('/search-mount.js')
      const {default:Component}=await import('/tests/fixtures/SearchWorkspace.svelte')
      window.component=mount(Component,{target:document.querySelector('#fixture')})
    })
    await page.getByRole('button',{name:'Search all mail',exact:true}).click()
    const search=page.getByRole('searchbox',{name:'Search all mail',exact:true})
    await search.fill('is:unread')
    // The failure surfaces as an error, not a spinner that never clears.
    await page.getByRole('alert').waitFor()
    assert.equal(await page.getByText('Searching...',{exact:true}).count(),0,'failed search must not leave the searching indicator up')
    // No retry or polling loop: backend traffic stays flat after the failure.
    const settle=()=>page.evaluate(()=>window.calls.length)
    const before=await settle()
    await page.waitForTimeout(800)
    assert.equal(await settle(),before,'a failed search must not retry or poll in the background')
    // Refining the query still works after the error.
    await search.fill('is:unread later')
    await page.getByRole('alert').waitFor()
    // Submitting routes to the list workspace, which shows its own error state.
    await search.press('Enter')
    await page.getByRole('heading',{name:'Search: is:unread later',exact:true}).waitFor()
    await page.getByText('Failed to load messages.',{exact:true}).waitFor()
    // The list still accepts commands: a filter change issues a fresh attempt.
    const attempts=()=>page.evaluate(()=>window.calls.filter(c=>c.name==='SearchMail').length)
    const prior=await attempts()
    await page.getByRole('button',{name:'Unread fixture filter',exact:true}).dispatchEvent('click')
    await page.waitForTimeout(500)
    const after=await attempts()
    assert.ok(after>prior&&after<20,'list stays interactive without piling up search calls')
    await page.screenshot({path:`${captureDir}/search-failure.png`})
    assert.deepEqual(errors.filter(e=>!e.includes('MailSearchRequest')),[],'no unhandled page errors beyond the surfaced search failure')
  }finally{
    await browser.close()
    await server.close()
  }
})
