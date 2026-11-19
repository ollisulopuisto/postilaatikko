import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync} from 'node:fs'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captureDir=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-list-load-error-captures'
mkdirSync(captureDir,{recursive:true})
// Regression: a folder load that fails (backend error or load timeout) showed
// only the generic "Failed to load messages." — the real cause was swallowed
// by the catch, so a wedged backend was indistinguishable from a bad query.
// The banner must carry the underlying error message.
test('a failing folder load surfaces the underlying error detail',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({root,plugins:[{name:'list-load-error-fixture',resolveId(id){if(id==='/list-mount.js')return id},load(id){if(id==='/list-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5199,strictPort:true},logLevel:'error'})
  await server.listen()
  const {chromium}=await import(browserModule)
  const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1280,height:900}})
    const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/list-load-error-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}))
    await page.goto('http://127.0.0.1:5199/list-load-error-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css')
      window.calls=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(target,key)=>target[key]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>(...args)=>{
        window.calls.push({name,args,at:performance.now()})
        if(name==='GetConversations')return Promise.reject(new Error('local mailbox service unavailable for conversation list: fixture backend wedged'))
        if(name==='GetFTSIndexStatus')return Promise.resolve({complete:true,totalMessages:0,indexedMessages:0})
        if(name==='IsFTSIndexing')return Promise.resolve(false)
        if(/Count/.test(name))return Promise.resolve(0)
        return Promise.resolve([])
      }})}}
      await (await import('/src/lib/i18n/index.ts')).initI18n('en')
      const {accountStore}=await import('/src/lib/stores/accounts.svelte.ts')
      accountStore.accounts=['a','b'].map(id=>({account:{id,name:`Account ${id}`,email:`${id}@example.test`,enabled:true,authType:'oauth2',imapHost:'imap.gmail.com'},folders:[],loading:false,syncing:false,error:null,lastSync:null}))
      const {mount}=await import('/list-mount.js')
      const {default:Component}=await import('/tests/fixtures/SearchWorkspace.svelte')
      window.component=mount(Component,{target:document.querySelector('#fixture')})
    })
    // The banner names the underlying failure instead of the generic text.
    const banner=page.locator('p.text-destructive').first()
    await banner.waitFor()
    const bannerText=await banner.textContent()
    assert.ok(bannerText.includes('fixture backend wedged'),`expected the backend error detail in the banner, got: ${bannerText}`)
    assert.ok(bannerText.includes('Failed to load messages'),`expected the generic prefix to stay, got: ${bannerText}`)
    // A retry re-issues the load (and surfaces the same detailed error).
    const prior=await page.evaluate(()=>window.calls.filter(c=>c.name==='GetConversations').length)
    await page.getByRole('button',{name:'Try again',exact:true}).click()
    await page.waitForTimeout(500)
    const after=await page.evaluate(()=>window.calls.filter(c=>c.name==='GetConversations').length)
    assert.ok(after>prior,'retry must re-issue the folder load')
    await page.getByText('fixture backend wedged',{exact:false}).waitFor()
    await page.screenshot({path:`${captureDir}/list-load-error.png`})
    assert.deepEqual(errors,[],'no unhandled page errors')
  }finally{
    await browser.close()
    await server.close()
  }
})
