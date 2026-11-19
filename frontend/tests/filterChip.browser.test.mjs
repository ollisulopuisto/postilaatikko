import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync} from 'node:fs'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captureDir=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-filter-chip-captures'
mkdirSync(captureDir,{recursive:true})
// The triage tab bar already shows the active Unread state ("Lukemattomat"
// highlighted), so a filter chip repeating "Filter: Unread x" is redundant.
// The chip row stays for list-origin filters (dropdown, keyboard) and for
// every other filter, where it is the only visible affordance.
test('triage-tab unread shows no filter chip; list-origin and other filters keep it',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({root,plugins:[{name:'filter-chip-fixture',resolveId(id){if(id==='/list-mount.js')return id},load(id){if(id==='/list-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5201,strictPort:true},logLevel:'error'})
  await server.listen()
  const {chromium}=await import(browserModule)
  const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1280,height:900}})
    const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/filter-chip-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}))
    await page.goto('http://127.0.0.1:5201/filter-chip-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css')
      window.calls=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(target,key)=>target[key]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>(...args)=>{
        window.calls.push({name,args,at:performance.now()})
        if(name==='GetConversations')return Promise.resolve([])
        if(name==='GetFTSIndexStatus')return Promise.resolve({complete:true,totalMessages:0,indexedMessages:0})
        if(name==='IsFTSIndexing')return Promise.resolve(false)
        if(/Count/.test(name))return Promise.resolve(0)
        return Promise.resolve([])
      }})}}
      await (await import('/src/lib/i18n/index.ts')).initI18n('en')
      const {accountStore}=await import('/src/lib/stores/accounts.svelte.ts')
      accountStore.accounts=['a','b'].map(id=>({account:{id,name:`Account ${id}`,email:`${id}@example.test`,enabled:true,authType:'oauth2',imapHost:'imap.gmail.com'},folders:[],loading:false,syncing:false,error:null,lastSync:null}))
      const {mount}=await import('/list-mount.js')
      const {default:Component}=await import('/tests/fixtures/FilterChipWorkspace.svelte')
      window.component=mount(Component,{target:document.querySelector('#fixture')})
    })
    const chipRow=page.locator('span',{hasText:'Filter:'})
    const lastListFilter=()=>page.evaluate(()=>{
      const calls=window.calls.filter(c=>c.name==='GetConversations')
      return calls.length?calls[calls.length-1].args[5]:null
    })

    // 1. Triage-tab unread: the filter applies (backend asked for unread) but
    // no redundant chip row renders.
    await page.getByRole('button',{name:'Triage unread',exact:true}).click()
    await page.waitForFunction(()=>window.calls.some(c=>c.name==='GetConversations'&&c.args[5]==='unread'))
    assert.equal(await lastListFilter(),'unread','triage unread must still filter the list')
    assert.equal(await chipRow.count(),0,'triage-tab unread must not render the filter chip row')

    // 2. Clearing removes the filter.
    await page.getByRole('button',{name:'Clear fixture',exact:true}).click()
    await page.waitForFunction(()=>{const c=window.calls.filter(x=>x.name==='GetConversations');return c.length&&c[c.length-1].args[5]===''})
    assert.equal(await chipRow.count(),0,'no chip without an active filter')

    // 3. List-origin unread (keyboard quick filter) keeps the chip: without
    // the tab bar's highlight it is the only visible affordance.
    await page.getByRole('button',{name:'Keyboard unread',exact:true}).click()
    await page.waitForFunction(()=>window.calls.some(c=>c.name==='GetConversations'&&c.args[5]==='unread'))
    await chipRow.first().waitFor()
    assert.ok(await chipRow.count()>=1,'list-origin unread keeps the filter chip')

    // 4. Other triage filters keep the chip (only unread is redundant with
    // the tab bar).
    await page.getByRole('button',{name:'Triage later',exact:true}).click()
    await page.waitForFunction(()=>window.calls.some(c=>c.name==='GetConversations'&&c.args[5]==='readLater'))
    await chipRow.first().waitFor()
    assert.ok(await chipRow.count()>=1,'readLater keeps the filter chip')

    // 5. Back to triage unread: hidden again, no stale row.
    await page.getByRole('button',{name:'Triage unread',exact:true}).click()
    await page.waitForFunction(()=>{const c=window.calls.filter(x=>x.name==='GetConversations');return c[c.length-1].args[5]==='unread'})
    assert.equal(await chipRow.count(),0,'returning to triage unread hides the chip again')

    await page.screenshot({path:`${captureDir}/filter-chip.png`})
    assert.deepEqual(errors,[],'no unhandled page errors')
  }finally{
    await browser.close()
    await server.close()
  }
})
