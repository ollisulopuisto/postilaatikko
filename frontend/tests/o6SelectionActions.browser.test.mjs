import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_O6_SCREENSHOT_DIR || '/tmp/posti-o6-captures'
test('O6 removes quick controls from every row during selection, including unresolved message IDs',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,cacheDir:join(root,'node_modules/.vite-o6'),plugins:[{name:'mount-o6',resolveId(id){if(id==='/o6-mount.js')return id},load(id){if(id==='/o6-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5217,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const {chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try {
    const page=await browser.newPage({viewport:{width:760,height:700}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/o6-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}));await page.goto('http://127.0.0.1:5217/o6-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts')
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(t,k)=>t[k]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:()=>async()=>[]})}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const {mount}=await import('/o6-mount.js');mount((await import('/tests/fixtures/O6Rows.svelte')).default,{target:document.querySelector('#fixture')})
    })
    const rows=page.locator('[data-conversation-row]');await rows.first().waitFor();assert.equal(await rows.count(),4)
    await rows.nth(1).hover();assert.equal(await page.locator('.quick-row-action').count(),12)
    await rows.nth(0).getByRole('checkbox').focus();await page.keyboard.press('Space');assert.equal(await page.locator('.quick-row-action').count(),0)
    for(const i of [0,1,3]){await rows.nth(i).hover();assert.equal(await page.locator('.quick-row-action').count(),0)}
    await rows.nth(2).getByRole('checkbox').click();await rows.nth(1).hover();await page.screenshot({path:join(captures,'o6-gap-selected.png'),fullPage:true})
    assert.equal(await page.locator('.quick-row-action').count(),0)
    assert.equal(await page.locator('.selection-run').count(),2)
    const boxes=await rows.evaluateAll(items=>items.map(row=>row.getBoundingClientRect().toJSON()))
    const bar=await page.locator('.selection-object-bar').boundingBox()
    assert.ok(bar.y>=boxes[2].y+45 && bar.y+bar.height<=boxes[2].bottom+1,'bar stays in reserved final-row footer')
    for(const width of [390,760]){await page.setViewportSize({width,height:700});await page.screenshot({path:join(captures,`o6-selected-${width}.png`),fullPage:true})}
    await page.getByRole('toolbar').getByRole('button',{name:'Archive',exact:true}).click();assert.equal(await page.evaluate(()=>window.fixtureAction),2)
    await page.getByRole('button',{name:'Clear fixture selection'}).click();await rows.nth(1).hover();assert.equal(await page.locator('.quick-row-action').count(),12)
    await page.screenshot({path:join(captures,'o6-no-selection.png'),fullPage:true});assert.deepEqual(errors,[])
  } finally {await browser.close();await server.close()}
})

test('actual MessageList wires O6 checked state and object geometry',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,cacheDir:join(root,'node_modules/.vite-test',String(process.pid),'o6integration'),plugins:[{name:'needs-me-mount',resolveId(id){if(id==='/needs-me-mount.js')return id},load(id){if(id==='/needs-me-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5218,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const {chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:640,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/needs-me-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}));await page.goto('http://127.0.0.1:5218/needs-me-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.calls=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(t,k)=>t[k]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>async(...args)=>{
        window.calls.push({name,args})
        if(name==='GetConversations')return [0,1,2].map(i=>({threadId:'sample'+i,accountId:'a',folderId:args[1],subject:'Synthetic row '+i,snippet:'Mail fixture',messageCount:1,unreadCount:0,participants:[{name:'Example sender',email:'sender@example.test'}],latestDate:'2026-11-20T12:00:00Z',messageIds:['sample-message'+i]}))
        if(name==='GetFTSIndexStatus')return {isComplete:true,totalCount:1,indexedCount:1}
        if(name==='IsFTSIndexing')return false
        if(/Count/.test(name))return 1
        return []
      }})}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const {mount}=await import('/needs-me-mount.js');const C=(await import('/tests/fixtures/NeedsMeWorkspace.svelte')).default
      mount(C,{target:document.querySelector('#fixture')})
    })
    const rows=page.locator('[data-conversation-row]');await rows.first().waitFor();assert.equal(await rows.count(),3)
    await rows.nth(0).getByRole('checkbox').focus();await page.keyboard.press('Space')
    await page.getByRole('toolbar',{name:'Actions for selected messages'}).waitFor()
    assert.equal(await page.locator('.quick-row-action').count(),0)
    await rows.nth(2).getByRole('checkbox').focus();await page.keyboard.press('Space')
    await page.waitForFunction(()=>document.querySelectorAll('.selection-run').length===2)
    assert.equal(await rows.nth(1).getAttribute('data-bulk-checked'),'false')
    await page.waitForFunction(()=>[...document.querySelectorAll('.quick-selection-checkbox')].every(el=>el.getBoundingClientRect().width>=23.5))
    await page.screenshot({path:join(captures,'o6-real-message-list.png'),fullPage:true})
    await page.getByRole('toolbar').getByRole('button',{name:'Close',exact:true}).click()
    assert.equal(await page.locator('.quick-row-action').count(),9)
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
