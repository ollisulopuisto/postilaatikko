import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_EXTENSIONS_CAPTURES||'/tmp/posti-extension-errors'
test('extension catalog failure is recoverable and toggle errors stay visible without optimistic state',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,cacheDir:join(root,'node_modules/.vite-test',String(process.pid),'extensionsFailure'),plugins:[{name:'extensions-mount',resolveId(id){if(id==='/extensions-mount.js')return id},load(id){if(id==='/extensions-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5207,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const{chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:390,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/extensions-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture" style="padding:20px"></div></body></html>'}));await page.goto('http://127.0.0.1:5207/extensions-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.failLoad=true;window.failToggle=true;window.enabled=true;window.toggleCalls=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(t,k)=>t[k]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>async(...args)=>{
        if(name==='ListExtensions'){if(window.failLoad)throw Error('private list detail');return [{id:'calendar',name:'Calendar',version:'1.0',enabled:window.enabled,description:'Calendar tools',capabilities:['calendar.read','calendar.manage']}]}
        if(name==='SetExtensionEnabled'){window.toggleCalls.push(args);if(window.failToggle)throw Error('private toggle detail');window.enabled=args[1];return}
        if(name==='ListEnabledExtensions')return window.enabled?['calendar']:[]
        return []
      }})}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en');const{mount}=await import('/extensions-mount.js');const Component=(await import('/src/lib/components/settings/ExtensionsTab.svelte')).default
      mount(Component,{target:document.querySelector('#fixture')})
    })
    const alert=page.getByRole('alert');await alert.waitFor({timeout:2500});assert.equal(await page.getByText('No extensions are available in this build.',{exact:true}).count(),0)
    await page.evaluate(()=>window.failLoad=false);await page.getByRole('button',{name:'Retry',exact:true}).click();await page.getByText('Calendar',{exact:true}).waitFor();assert.equal(await alert.count(),0)
    await page.getByRole('button',{name:'Disable',exact:true}).click();await alert.waitFor();assert.equal(await page.getByRole('button',{name:'Disable',exact:true}).isEnabled(),true);assert.deepEqual(await page.evaluate(()=>window.toggleCalls),[['calendar',false]])
    assert.equal((await page.locator('body').innerText()).includes('private toggle detail'),false)
    for(const lang of ['en','fi']){
      await page.evaluate(async lang=>{await(await import('/src/lib/i18n/index.ts')).initI18n(lang)},lang)
      for(const width of [320,390,1024]){
        await page.setViewportSize({width,height:900});await page.screenshot({path:join(captures,`extensions-toggle-error-${lang}-${width}.png`),fullPage:true})
        assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),'no horizontal overflow')
      }
    }
    await page.evaluate(async()=>{await(await import('/src/lib/i18n/index.ts')).initI18n('en');window.failToggle=false})
    await page.getByRole('button',{name:'Disable',exact:true}).click();await page.getByRole('button',{name:'Enable',exact:true}).waitFor();assert.equal(await alert.count(),0);assert.deepEqual(await page.evaluate(()=>window.toggleCalls),[['calendar',false],['calendar',false]]);await page.evaluate(()=>window.failLoad=true)
    await page.getByRole('button',{name:'Enable',exact:true}).click();await alert.waitFor();await page.getByText('Calendar',{exact:true}).waitFor()
    await page.evaluate(()=>window.failLoad=false);await page.getByRole('button',{name:'Retry',exact:true}).click();await page.getByRole('button',{name:'Disable',exact:true}).waitFor();assert.equal(await alert.count(),0)
    assert.deepEqual(await page.evaluate(()=>window.toggleCalls),[['calendar',false],['calendar',false],['calendar',true]],'load retry never repeats the toggle');assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
