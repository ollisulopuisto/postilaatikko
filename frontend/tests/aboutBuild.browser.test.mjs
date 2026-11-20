import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync,realpathSync} from 'node:fs'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captureDir=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-about-build-captures'
mkdirSync(captureDir,{recursive:true})
test('About shows the exact build and hides absent build metadata',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({root,plugins:[{name:'about-build-fixture',resolveId(id){if(id==='/about-mount.js')return id},load(id){if(id==='/about-mount.js')return `export {mount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5198,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen()
  const {chromium}=await import(browserModule)
  const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    for(const build of ['2026.11.19.1-test.44518590233.1+8c95950e1234','','retry']){
      const page=await browser.newPage({viewport:{width:700,height:900}})
      const errors=[];page.on('pageerror',e=>errors.push(e.message))
      await page.route('**/about-build-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>'}))
      await page.goto('http://127.0.0.1:5198/about-build-fixture')
      await page.evaluate(async(build)=>{
        await import('/src/app.css')
        window.runtime=new Proxy({EventsOnMultiple:()=>()=>{}},{get:(target,key)=>target[key]||(()=>Promise.resolve())})
        let attempts=0
        window.go={app:{App:new Proxy({},{get:(_,name)=>()=>{
          if(name==='GetAppInfo' && build==='retry' && attempts++===0)return Promise.reject(new Error('temporary app-info failure'))
          if(name==='GetAppInfo')return Promise.resolve({name:'Postilaatikko',version:'2026.11.19.1',build,description:'A personal desktop email client for multiple accounts',website:'https://example.test',license:'Apache 2.0'})
          if(name==='GetAutoCheckUpdates')return Promise.resolve(false)
          return Promise.resolve('')
        }})}}
        await (await import('/src/lib/i18n/index.ts')).initI18n('en')
        const {mount}=await import('/about-mount.js')
        const {default:Component}=await import('/tests/fixtures/AboutBuild.svelte')
        window.component=mount(Component,{target:document.querySelector('#fixture')})
      },build)
      if(build==='retry'){
        await page.getByText('Failed to load app information',{exact:true}).waitFor()
        for(const width of [320,390,700]){
          await page.setViewportSize({width,height:900})
          const retry=page.getByRole('button',{name:'Retry',exact:true})
          const box=await retry.boundingBox()
          assert.ok(box && box.x>=0 && box.x+box.width<=width,'retry stays inside viewport')
          await page.screenshot({path:`${captureDir}/about-load-error-${width}.png`})
        }
        await page.getByRole('button',{name:'Retry',exact:true}).click({timeout:2000})
      }
      await page.getByText('Version 2026.11.19.1',{exact:true}).waitFor()
      if(build && build!=='retry'){
        assert.equal(await page.getByTestId('about-build').textContent().then(s=>s.trim()),`Build ${build}`)
        await page.screenshot({path:`${captureDir}/about-build.png`})
      }else if(!build) assert.equal(await page.getByTestId('about-build').count(),0)
      assert.deepEqual(errors,[],'no unhandled page errors')
      await page.close()
    }
  }finally{await browser.close();await server.close()}
})
