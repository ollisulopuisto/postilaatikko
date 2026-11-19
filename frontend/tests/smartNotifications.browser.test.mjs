import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-notifications-captures'
test('smart notification controls load, save rules and preserve failed-load state',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,plugins:[{name:'mount-helper',resolveId(id){if(id==='/rescue-mount.js')return id},load(id){if(id==='/rescue-mount.js')return `export {mount,unmount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5195,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen();const{chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:process.env.POSTI_CHROMIUM_PATH,headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:940,height:700}});const errors=[];page.on('pageerror',e=>errors.push(e.message))
    await page.route('**/rescue-fixture',route=>route.fulfill({contentType:'text/html',body:'<html data-theme="light"><body><div id="fixture" style="padding:24px"></div></body></html>'}));await page.goto('http://127.0.0.1:5195/rescue-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css');await import('/src/lib/iconify-offline.ts');window.prefs={enabled:true,rules:[]};window.saves=[];window.failLoad=false;window.failSave=false
      window.go={app:{App:{GetSmartNotificationSettings:async()=>{if(window.failLoad)throw Error('fixture unavailable');return structuredClone(window.prefs)},SaveSmartNotificationSettings:async p=>{if(window.failSave)throw Error('fixture write failed');window.prefs=structuredClone(p);window.saves.push(p)}}}}
      await(await import('/src/lib/i18n/index.ts')).initI18n('en')
      const{mount,unmount}=await import('/rescue-mount.js');const Component=(await import('/src/lib/components/settings/SmartNotifications.svelte')).default
      window.show=async()=>{if(window.component)await unmount(window.component);window.component=mount(Component,{target:document.querySelector('#fixture')})};await window.show()
    })
    const input=page.locator('input:not([type=checkbox])');await input.waitFor();await input.fill('muted@example.com');await page.locator('button[type=submit]').click();await page.waitForTimeout(100)
    assert.equal(await page.evaluate(()=>window.saves.at(-1).rules[0].match),'muted@example.com')
    assert.equal(await page.getByText('muted@example.com',{exact:true}).locator('..').getByRole('combobox').count(),1,'saved rule scope is fixed; only the action is editable')
    await page.screenshot({path:join(captures,'notifications-en.png'),fullPage:true})
    await page.evaluate(async()=>{document.documentElement.dataset.theme='dark';await(await import('/src/lib/i18n/index.ts')).initI18n('fi')})
    await page.waitForTimeout(100);await page.screenshot({path:join(captures,'notifications-fi-dark.png'),fullPage:true})
    await page.evaluate(async()=>{document.documentElement.dataset.theme='light';await(await import('/src/lib/i18n/index.ts')).initI18n('en')})

    await page.evaluate(()=>window.failSave=true);await page.getByRole('switch').click();await page.waitForTimeout(100);assert.equal(await page.getByRole('switch').getAttribute('aria-checked'),'true')
    await page.evaluate(async()=>{window.failLoad=true;await window.show()});await page.waitForTimeout(150);assert.equal(await page.locator('button[type=submit]').isDisabled(),true);assert.equal(await page.getByRole('switch').isDisabled(),true)
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
