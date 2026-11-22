import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
// The triage tab strip ("Kaikki | Odottaa minua | Lukemattomat | Myöhemmin")
// must fit the list pane: tabs wrap to a second row instead of forcing a
// horizontal scrollbar. Finnish labels are long, so a narrow pane used to
// overflow the strip (scrollWidth 351 > clientWidth 267 at 320 px).
test('triage tab strip wraps instead of overflowing horizontally',{skip:!browserModule,timeout:90000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({root,cacheDir:join(root,'node_modules/.vite-test',String(process.pid),'triageTabs'),server:{host:'127.0.0.1',port:5211,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const {chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1280,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.addInitScript(()=>{
      localStorage.setItem('postilaatikko:context-pane-auto-hide','false')
      window.listeners=new Map();window.states=[];window.backendCalls=[]
      window.runtime=new Proxy({}, {get:(_,key)=>key==='EventsOnMultiple'?(name,callback)=>{const list=window.listeners.get(name)||[];list.push(callback);window.listeners.set(name,list);return()=>window.listeners.set(name,list.filter(cb=>cb!==callback))}:key==='EventsEmit'?(name,state)=>{if(name==='native-menu:state')window.states.push(state)}:()=>Promise.resolve()})
      window.data={};const data=window.data;Object.assign(data,{GetAppInfo:{version:'2026.11.22.1'},GetLastSeenWhatsNewVersion:'2026.11.22.1',GetComposeMode:{accountId:'demo',mode:'new'},GetWindowDecorationStatus:{effective_mode:'native'},DraftHistoryTree:{nodes:[],draftKey:'fixture',headNodeId:''},IsReady:true,GetThemeMode:'dark',GetLanguage:'fi',GetAccounts:[{id:'demo',name:'Demo',email:'demo@example.test',type:'imap',enabled:true}],GetUIState:{selectedAccountId:'unified',selectedFolderId:'inbox',selectedFolderName:'Inbox',selectedFolderType:'inbox'},GetUnifiedFolderCount:1,GetUnifiedFolderConversations:[{threadId:'thread1',accountId:'demo',folderId:'inbox',subject:'Fixture subject',snippet:'Fixture snippet',latestDate:new Date().toISOString(),messageCount:1,unreadCount:0,participants:[{name:'Fixture sender',email:'fixture@example.test'}],messageIds:['m1']}]})
      const backend=new Proxy({}, {get:(_,key)=>(...args)=>{window.backendCalls.push([key,args]);return Promise.resolve(key==='ResolveMailActionTarget'?args[0].messages.map(message=>message.messageId):key in data?data[key]:key.startsWith('Get')||key.startsWith('List')?[]:null)}})
      window.go={app:{App:backend,ComposerApp:backend,ReaderApp:{GetConfig:async()=>null,Call:(operation,args)=>backend[operation](...args)}}}
    })
    await page.goto('http://127.0.0.1:5211',{waitUntil:'domcontentloaded'})
    await page.locator('[data-mail-row-key]').first().waitFor({timeout:30000})
    const strips=()=>page.evaluate(()=>[...document.querySelectorAll('div[role="group"]')]
      .filter(group=>group.textContent.includes('Odottaa minua')&&group.clientWidth>0)
      .map(group=>({scrollWidth:group.scrollWidth,clientWidth:group.clientWidth})))
    for(const width of [320,480,640,1280]){
      await page.setViewportSize({width,height:900})
      await page.waitForTimeout(300)
      const fits=await strips()
      assert.ok(fits.length>0,`triage strip must render at ${width}px`)
      for(const fit of fits) assert.ok(fit.scrollWidth<=fit.clientWidth+1,`triage strip overflows at ${width}px: scrollWidth ${fit.scrollWidth} > clientWidth ${fit.clientWidth}`)
    }
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
