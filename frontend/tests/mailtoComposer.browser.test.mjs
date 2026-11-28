import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync} from 'node:fs'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-mailto-captures'
test('rich composer displays text-only mailto body literally, without loading HTML',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,plugins:[{name:'mount',resolveId:id=>id==='/test-mount.js'?id:null,load:id=>id==='/test-mount.js'?`export {mount} from 'svelte'`:null}],server:{host:'127.0.0.1',port:5397,strictPort:true},logLevel:'error'})
  await server.listen();const{chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome',args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1100,height:800}});let remote=false;page.on('request',r=>{if(r.url().includes('evil.example'))remote=true})
    await page.route('**/mailto-fixture',r=>r.fulfill({contentType:'text/html',body:'<html data-theme="dark" class="dark"><body class="bg-background text-foreground"><div id="fixture"></div></body></html>'}));await page.goto('http://127.0.0.1:5397/mailto-fixture')
    await page.evaluate(async()=>{
      window.runtime={EventsOnMultiple:()=>()=>{},EventsOff:()=>{}};window.go={app:{App:new Proxy({}, {get:()=>async()=>[]})}};await import('/src/app.css');await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const api=new Proxy({getIdentities:async()=>[],getAllAccountIdentities:async()=>[],getAccount:async()=>({}),draftHistoryTree:async()=>({nodes:[]}),draftHistoryState:async()=>'',draftHistoryCommit:async()=>'',getSendLaterSetup:async()=>({configured:false})},{get:(t,k)=>t[k]|| (async()=>[])})
      const{mount}=await import('/test-mount.js');const Composer=(await import('/src/lib/components/composer/Composer.svelte')).default
      mount(Composer,{target:document.querySelector('#fixture'),props:{accountId:'fixture',api,initialMessage:{to:[{address:'to@example.test'}],cc:[{address:'cc@example.test'}],bcc:[{address:'bcc@example.test'}],subject:'Postilaatikko mailto test',text_body:'Sent from Postilaatikko via a mailto link\n<img src="https://evil.example/image"><script>evil()</script>',html_body:'',attachments:[]}}})
    })
    const editor=page.locator('.composer-editor');await editor.waitFor();await page.waitForTimeout(500);assert.ok((await editor.innerText()).includes('Sent from Postilaatikko via a mailto link'));assert.ok((await editor.innerText()).includes('<img src='));assert.equal(await editor.locator('img,script').count(),0);assert.equal(remote,false);assert.ok(await page.getByText('bcc@example.test').first().isVisible(),'prefilled Bcc must be visible');await page.screenshot({path:captures+'/mailto-rich-body.png'})
  }finally{await browser.close();await server.close()}
})
