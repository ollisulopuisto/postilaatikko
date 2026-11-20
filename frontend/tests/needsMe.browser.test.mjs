import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures='/tmp/posti-needs-me-captures'
test('Needs me remains an unread-or-Later backend filter through folder switches',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,cacheDir:join(root,'node_modules/.vite-test',String(process.pid),'needsMe'),plugins:[{name:'needs-me-mount',resolveId(id){if(id==='/needs-me-mount.js')return id},load(id){if(id==='/needs-me-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5208,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const {chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:640,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/needs-me-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}));await page.goto('http://127.0.0.1:5208/needs-me-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.calls=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(t,k)=>t[k]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>async(...args)=>{
        window.calls.push({name,args})
        if(name==='GetConversations')return [{threadId:'sample',accountId:'a',folderId:args[1],subject:args[5]==='needsMe'?'Read, saved for later':'Other mail',snippet:'Synthetic example',messageCount:1,unreadCount:0,participants:[{name:'Example sender',email:'sender@example.test'}],latestDate:'2026-11-20T12:00:00Z',messageIds:['sample-message']}]
        if(name==='GetFTSIndexStatus')return {isComplete:true,totalCount:1,indexedCount:1}
        if(name==='IsFTSIndexing')return false
        if(/Count/.test(name))return 1
        return []
      }})}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const {mount}=await import('/needs-me-mount.js');const C=(await import('/tests/fixtures/NeedsMeWorkspace.svelte')).default
      mount(C,{target:document.querySelector('#fixture')})
    })
    const last=()=>page.evaluate(()=>window.calls.filter(x=>x.name==='GetConversations').at(-1)?.args)
    await page.getByText('Read, saved for later',{exact:true}).waitFor();assert.equal((await last())[5],'needsMe')
    await page.getByRole('button',{name:'Projects fixture',exact:true}).click()
    await page.waitForFunction(()=>window.calls.filter(x=>x.name==='GetConversations').at(-1)?.args[1]==='projects-a')
    assert.equal((await last())[5],'needsMe','folder change retains mode')
    await page.getByRole('button',{name:'All',exact:true}).click()
    await page.getByText('Other mail',{exact:true}).waitFor();assert.equal((await last())[5],'')
    await page.getByRole('button',{name:'Needs me',exact:true}).click()
    await page.getByText('Read, saved for later',{exact:true}).waitFor()
    for(const lang of ['en','fi']){
      await page.evaluate(async lang=>{await(await import('/src/lib/i18n/index.ts')).initI18n(lang)},lang)
      for(const width of [320,640]){
        await page.setViewportSize({width,height:900});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
        await page.screenshot({path:join(captures,`needs-me-${lang}-${width}.png`),fullPage:true})
      }
    }
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})


test('real App retains Needs me through folder navigation and offers aggregate scope',{skip:!browserModule,timeout:90000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({root,cacheDir:join(captures,'app-cache'),server:{host:'127.0.0.1',port:5209,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const {chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1280,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.addInitScript(()=>{
      localStorage.setItem('postilaatikko:context-pane-auto-hide','false')
      window.listeners=new Map();window.states=[];window.backendCalls=[]
      window.runtime=new Proxy({}, {get:(_,key)=>key==='EventsOnMultiple'?(name,callback)=>{const list=window.listeners.get(name)||[];list.push(callback);window.listeners.set(name,list);return()=>window.listeners.set(name,list.filter(cb=>cb!==callback))}:key==='EventsEmit'?(name,state)=>{if(name==='native-menu:state')window.states.push(state)}:()=>Promise.resolve()})
      window.data={};const data=window.data;Object.assign(data,{GetAppInfo:{version:'2026.11.19.1'},GetLastSeenWhatsNewVersion:'2026.11.19.1',GetComposeMode:{accountId:'demo',mode:'new'},GetWindowDecorationStatus:{effective_mode:'native'},DraftHistoryTree:{nodes:[],draftKey:'fixture',headNodeId:''},IsReady:true,GetThemeMode:'dark',GetLanguage:'en',GetAccounts:[{id:'demo',name:'Demo',email:'demo@example.test',type:'imap',enabled:true}],GetUIState:{selectedAccountId:'unified',selectedFolderId:'inbox',selectedFolderName:'Inbox',selectedFolderType:'inbox'},GetUnifiedFolderCount:1,GetUnifiedFolderConversations:[{threadId:'thread1',accountId:'demo',folderId:'inbox',subject:'Needs me example',snippet:'Synthetic triage example',latestDate:new Date().toISOString(),messageCount:1,unreadCount:0,participants:[{name:'Fixture sender',email:'fixture@example.test'}]}],GetConversation:{id:'thread1',subject:'Needs me example',messages:[{id:'msg1',accountId:'demo',folderId:'inbox',threadId:'thread1',subject:'Needs me example',fromName:'Fixture sender',fromEmail:'fixture@example.test',toList:[],ccList:[],date:new Date().toISOString(),bodyText:'Reading does not remove a Later mark.',isRead:true}]}})
      const backend=new Proxy({}, {get:(_,key)=>(...args)=>{window.backendCalls.push([key,args]);return Promise.resolve(key==='ResolveMailActionTarget'?args[0].messages.map(message=>message.messageId):key in data?data[key]:key.startsWith('Get')||key.startsWith('List')?[]:null)}})
      window.go={app:{App:backend,ComposerApp:backend,ReaderApp:{GetConfig:async()=>({accountId:'demo',folderId:'inbox',folderName:'Inbox',folderType:'inbox',threadId:'thread1',threadAccountId:'demo',threadFolderId:'inbox'}),Call:(operation,args)=>backend[operation](...args)}}}
    })

    await page.goto('http://127.0.0.1:5209',{waitUntil:'domcontentloaded'})
    await page.locator('[data-mail-row-key]').first().waitFor({timeout:30000})
    await page.getByRole('button',{name:'Needs me',exact:true}).first().click()
    await page.waitForFunction(()=>window.backendCalls.some(([name,args])=>name==='GetUnifiedFolderConversations'&&args[4]==='needsMe'))
    // Call the existing app navigation event, the same boundary used by palette/pins.
    await page.evaluate(()=>window.dispatchEvent(new CustomEvent('postilaatikko:navigate',{detail:{id:'projects-fixture',kind:'folder',name:'Projects',accountId:'demo',folderId:'projects',path:'Projects'}})))
    await page.waitForFunction(()=>window.backendCalls.some(([name,args])=>name==='GetConversations'&&args[1]==='projects'&&args[5]==='needsMe'))
    await page.evaluate(()=>window.dispatchEvent(new CustomEvent('postilaatikko:navigate',{detail:{id:'triage',kind:'system',name:'All folders',folderType:'triage'}})))
    await page.waitForFunction(()=>window.backendCalls.some(([name,args])=>name==='GetUnifiedFolderConversations'&&args[0]==='triage'&&args[4]==='needsMe'))
    await page.getByText('All enabled accounts. Excludes Spam, Sent, Drafts, Archive, Trash, All Mail and hidden folders. Snoozed mail returns when due.',{exact:true}).waitFor()
    for(const lang of ['en','fi']){
      await page.evaluate(async lang=>{await(await import('/src/lib/i18n/index.ts')).initI18n(lang)},lang)
      await page.screenshot({path:join(captures,`needs-me-real-app-${lang}.png`),fullPage:true})
    }
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
