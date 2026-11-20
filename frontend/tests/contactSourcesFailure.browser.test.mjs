import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_CONTACTS_CAPTURES||'/tmp/posti-contacts-errors'
test('contact source load failure is not an empty list and retry retains last good sources',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,plugins:[{name:'contacts-mount',resolveId(id){if(id==='/contacts-mount.js')return id},load(id){if(id==='/contacts-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5205,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const{chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:390,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/contacts-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture" style="padding:20px"></div></body></html>'}));await page.goto('http://127.0.0.1:5205/contacts-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.failLoad=true
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(t,k)=>t[k]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>async()=>{
        if(name==='GetContactSources'){if(window.failLoad)throw Error('source catalog unavailable');return [{id:'contacts-1',name:'Work contacts',type:'carddav',enabled:true,writable:false,url:'https://example.test/contacts',last_error:''}]}
        if(name==='GetContactSourceErrors')return []
        return []
      }})}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const{mount}=await import('/contacts-mount.js');const Component=(await import('/src/lib/components/settings/ContactSources.svelte')).default
      mount(Component,{target:document.querySelector('#fixture')})
    })
    const alert=page.getByRole('alert')
    await alert.waitFor({timeout:2500})
    assert.equal(await page.getByText('No contact sources configured',{exact:true}).count(),0)
    await page.evaluate(()=>window.failLoad=false)
    await page.getByRole('button',{name:'Retry',exact:true}).click()
    await page.getByText('Work contacts',{exact:true}).waitFor()
    assert.equal(await alert.count(),0)
    await page.evaluate(async()=>{window.failLoad=true;await(await import('/src/lib/stores/contactSources.svelte.ts')).contactSourcesStore.refresh()})
    await alert.waitFor();await page.getByText('Work contacts',{exact:true}).waitFor()
    for(const lang of ['en','fi']){
      await page.evaluate(async lang=>{await(await import('/src/lib/i18n/index.ts')).initI18n(lang)},lang)
      for(const width of [320,390,1024]){
        await page.setViewportSize({width,height:900});await page.waitForTimeout(100)
        const box=await alert.boundingBox();assert.ok(box.x>=0&&box.x+box.width<=width)
        assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),'source actions must not overflow the viewport')
        await page.screenshot({path:join(captures,`contacts-load-error-${lang}-${width}.png`),fullPage:true})
      }
    }
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
