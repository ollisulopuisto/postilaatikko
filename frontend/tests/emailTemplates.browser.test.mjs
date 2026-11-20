import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-template-captures'
for (const detached of [false, true]) test(`templates save, insert subject/body and release the dialog guard (${detached ? 'detached' : 'main'})`,{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({cacheDir:fileURLToPath(new URL(`../node_modules/.vite-test/${process.pid}/emailTemplates`,import.meta.url)),root,plugins:[{name:'mount-helper',resolveId(id){if(id==='/rescue-mount.js')return id},load(id){if(id==='/rescue-mount.js')return `export {mount,unmount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5196,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const{chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:process.env.POSTI_CHROMIUM_PATH,headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:940,height:800}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/rescue-fixture',route=>route.fulfill({contentType:'text/html',body:'<html data-theme="light"><body><div id="fixture" style="padding:24px"></div></body></html>'}));await page.goto('http://127.0.0.1:5196/rescue-fixture')
    await page.evaluate(async(detached)=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.templates=[];window.inserted=[]
      window.go={app:{[detached ? 'ComposerApp' : 'App']:{ListEmailTemplates:async()=>structuredClone(window.templates),SaveEmailTemplate:async t=>{const result={...t,id:t.id||'saved'};window.templates=[result];return result},DeleteEmailTemplate:async()=>{window.templates=[]}}}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const{mount,unmount}=await import('/rescue-mount.js');window.unmount=unmount;const Component=(await import('/src/lib/components/composer/EmailTemplates.svelte')).default
      const apiModule=await import('/src/lib/composerApi.ts');const api=detached?apiModule.createComposerWindowApi('test-account'):apiModule.createMainWindowApi()
      window.component=mount(Component,{target:document.querySelector('#fixture'),props:{api,onInsert:t=>window.inserted.push(t)}})
    },detached)
    await page.getByRole('button',{name:'Email templates',exact:true}).click();await page.getByRole('menuitem').last().click();const dialog=page.getByRole('dialog');await dialog.waitFor()
    await dialog.locator('input').nth(0).fill('Greeting');await dialog.locator('input').nth(1).fill('Hello');await dialog.locator('textarea').fill('First line\nSecond line');await dialog.getByRole('button',{name:'Add',exact:true}).click();await page.getByText('Greeting',{exact:true}).waitFor()
    await page.screenshot({path:join(captures,`templates-manager-${detached?'detached':'main'}-en.png`),fullPage:true})
    const box=await dialog.boundingBox();const close=await dialog.getByRole('button',{name:'Close',exact:true}).boundingBox()
    assert.ok(close.x>=box.x&&close.x+close.width<=box.x+box.width,'close control stays inside the template dialog')
    await page.evaluate(async()=>{document.documentElement.dataset.theme='dark';await(await import('/src/lib/i18n/index.ts')).initI18n('fi')})
    await page.waitForTimeout(100);await page.screenshot({path:join(captures,`templates-manager-${detached?'detached':'main'}-fi-dark.png`),fullPage:true})
    await page.evaluate(async()=>{document.documentElement.dataset.theme='light';await(await import('/src/lib/i18n/index.ts')).initI18n('en')})

    await page.keyboard.press('Escape');await page.getByRole('button',{name:'Email templates',exact:true}).click();await page.getByRole('menuitem').filter({hasText:'Greeting'}).click();assert.equal(await page.evaluate(()=>window.inserted[0].body),'First line\nSecond line')
    await page.getByRole('button',{name:'Email templates',exact:true}).click();await page.getByRole('menuitem').last().click();await dialog.waitFor()
    await dialog.getByRole('button',{name:'Delete',exact:true}).click();await page.waitForFunction(()=>window.templates.length===0);await page.getByText('Greeting',{exact:true}).waitFor({state:'hidden'})
    assert.equal(await page.evaluate(async()=> (await import('/src/lib/stores/dialogGuard.ts')).isDialogGuardActive()),true)
    await page.evaluate(async()=>window.unmount(window.component));assert.equal(await page.evaluate(async()=> (await import('/src/lib/stores/dialogGuard.ts')).isDialogGuardActive()),false)
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
