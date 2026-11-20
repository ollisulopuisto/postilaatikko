import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync, realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_IMAGES_CAPTURES||'/tmp/posti-images-errors'
test('image allowlist failures stay visible, preserve data and support explicit recovery',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,cacheDir:join(root,'node_modules/.vite-test',String(process.pid),'imagesFailure'),plugins:[{name:'images-mount',resolveId(id){if(id==='/images-mount.js')return id},load(id){if(id==='/images-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5206,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const {chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:390,height:900}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/images-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture" style="padding:20px"></div></body></html>'}));await page.goto('http://127.0.0.1:5206/images-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.failLoad=true;window.failRemove=true;window.removeCalls=0;window.loadCalls=0;window.privacyChanges=[]
      window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(t,k)=>t[k]||(()=>Promise.resolve())})
      window.go={app:{App:new Proxy({},{get:(_,name)=>async()=>{
        if(name==='GetImageAllowlist'){window.loadCalls++;if(window.failLoad)throw Error('private backend detail');return window.removed?[]:[{id:1,type:'sender',value:'sender@example.test'}]}
        if(name==='RemoveImageAllowlist'){window.removeCalls++;if(window.failRemove)throw Error('private removal detail');window.removed=true;return}
        return []
      }})}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en');const {mount}=await import('/images-mount.js');const Component=(await import('/src/lib/components/settings/ImagesTab.svelte')).default
      mount(Component,{target:document.querySelector('#fixture'),props:{alwaysLoadImages:false,onAlwaysLoadImagesChange:value=>window.privacyChanges.push(value)}})
    })
    const alert=page.getByRole('alert');await alert.waitFor({timeout:2500})
    assert.equal(await page.getByText('No addresses in the allowlist. Use "Always Load > For sender" in the email viewer to add addresses.',{exact:true}).count(),0)
    assert.equal(await page.getByText('No domains in the allowlist. Use "Always Load > For domain" in the email viewer to add domains.',{exact:true}).count(),0)
    assert.equal((await page.locator('body').innerText()).includes('private backend detail'),false);await page.screenshot({path:join(captures,'images-load-error-en-390.png'),fullPage:true})
    await page.evaluate(()=>window.failLoad=false);await page.getByRole('button',{name:'Retry',exact:true}).click();await page.getByText('sender@example.test',{exact:true}).waitFor();assert.equal(await alert.count(),0)
    const remove=page.getByRole('button',{name:'Remove from allowlist',exact:true});await remove.click();await alert.waitFor();await page.getByText('sender@example.test',{exact:true}).waitFor();assert.equal(await page.evaluate(()=>window.removeCalls),1)
    assert.equal((await page.locator('body').innerText()).includes('private removal detail'),false)
    for(const lang of ['en','fi']){
      await page.evaluate(async lang=>{await(await import('/src/lib/i18n/index.ts')).initI18n(lang)},lang)
      for(const width of [320,390,1024]){
        await page.setViewportSize({width,height:900});await page.evaluate(()=>window.scrollTo(0,0));await page.screenshot({path:join(captures,`images-remove-error-${lang}-${width}.png`),fullPage:true})
        assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),'no horizontal overflow');const toggle=await page.getByRole('switch').boundingBox();assert.ok(toggle.width>=40&&toggle.x+toggle.width<=width,'privacy toggle remains full width')
      }
    }
    await page.evaluate(async()=>{await(await import('/src/lib/i18n/index.ts')).initI18n('en');window.failRemove=false;window.failLoad=true})
    await remove.click();await alert.waitFor();assert.equal(await page.evaluate(()=>window.removeCalls),2)
    assert.equal((await alert.innerText()).includes('removed'),false,'refresh failure must not claim the removal failed or succeeded')
    await page.evaluate(()=>window.failLoad=false);await page.getByRole('button',{name:'Retry',exact:true}).click();await alert.waitFor({state:'detached'});assert.equal(await page.getByText('sender@example.test',{exact:true}).count(),0)
    assert.equal(await page.evaluate(()=>window.removeCalls),2,'refresh retry must not repeat the mutation');assert.deepEqual(await page.evaluate(()=>window.privacyChanges),[]);assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
