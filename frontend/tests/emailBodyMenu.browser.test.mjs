import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {fileURLToPath} from 'node:url'
import {mkdirSync} from 'node:fs'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captures=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-menu-captures'
test('body link menu and URL tooltip stay within the window at edges and after resize',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url));mkdirSync(captures,{recursive:true})
  const server=await createServer({root,plugins:[{name:'mount',resolveId:id=>id==='/test-mount.js'?id:null,load:id=>id==='/test-mount.js'?`export {mount,unmount} from 'svelte'`:null}],server:{host:'127.0.0.1',port:5396,strictPort:true},logLevel:'error'})
  await server.listen();const{chromium}=await import(browserModule);const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome',args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:940,height:700}})
    await page.route('**/menu-fixture',r=>r.fulfill({contentType:'text/html',body:'<html class="dark" data-theme="dark"><body class="bg-background text-foreground"><div id="fixture" style="position:absolute;left:450px;top:150px;width:450px"></div></body></html>'}))
    await page.goto('http://127.0.0.1:5396/menu-fixture')
    await page.evaluate(async()=>{
      window.go={app:{App:new Proxy({}, {get:()=>async()=>[]})}};await import('/src/app.css');await(await import('/src/lib/i18n/index.ts')).initI18n('fi');const{mount}=await import('/test-mount.js');const Body=(await import('/src/lib/components/viewer/EmailBody.svelte')).default
      window.component=mount(Body,{target:document.querySelector('#fixture'),props:{messageId:'fixture',accountId:'a',conversationId:'c',bodyHtml:'<a href="https://example.com/path?utm_source=mail">Link</a>'}})
    })
    const iframe=page.locator('iframe');await iframe.waitFor();await page.waitForTimeout(500)
    const send=async(type,x,y)=>page.evaluate(({type,x,y})=>{const f=document.querySelector('iframe');window.dispatchEvent(new MessageEvent('message',{source:f.contentWindow,data:{type,url:'https://example.com/a/very/long/path?utm_source=mail',text:'selected',x:x-f.getBoundingClientRect().left,y:y-f.getBoundingClientRect().top}}))},{type,x,y})
    await send('link-hover',930,690);await page.waitForTimeout(100)
    const tip=page.locator('.pointer-events-none').filter({hasText:'https://example.com/a/very/long/path'})
    await tip.waitFor();let box=await tip.boundingBox();assert.ok(box.x>=0&&box.y>=0&&box.x+box.width<=940&&box.y+box.height<=700,JSON.stringify(box))
    await send('contextmenu',930,690);const menu=page.getByRole('menu');await menu.waitFor();await page.waitForTimeout(100)
    await page.screenshot({path:captures+'/menu-bottom-right.png'})
    box=await menu.boundingBox();assert.ok(box.x>=0&&box.y>=0&&box.x+box.width<=940&&box.y+box.height<=700,JSON.stringify(box));assert.equal(await tip.count(),0)
    await page.setViewportSize({width:360,height:240});await page.waitForTimeout(100);box=await menu.boundingBox();assert.ok(box.x>=8&&box.y>=8&&box.x+box.width<=352&&box.y+box.height<=232,JSON.stringify(box));assert.ok(await menu.evaluate(n=>n.scrollHeight>=n.clientHeight))
    await page.screenshot({path:captures+'/menu-small-window.png'})
    await menu.locator('button').last().scrollIntoViewIfNeeded();const last=await menu.locator('button').last().boundingBox();assert.ok(last.y+last.height<=232,'last action is reachable by scrolling')
    await page.getByLabel('Sulje pikavalikko').click({position:{x:2,y:2}})
    await page.setViewportSize({width:940,height:700})
    // Exercise the plain-text DOM path as well as iframe coordinate translation.
    await page.evaluate(async()=>{
      const{unmount,mount}=await import('/test-mount.js');await unmount(window.component);const Body=(await import('/src/lib/components/viewer/EmailBody.svelte')).default
      window.component=mount(Body,{target:document.querySelector('#fixture'),props:{messageId:'plain',bodyText:'https://example.com/plain'}})
    })
    const link=page.locator('#fixture a');await link.waitFor();await link.dispatchEvent('contextmenu',{clientX:938,clientY:698,button:2,bubbles:true});await menu.waitFor();await page.waitForTimeout(100);box=await menu.boundingBox();assert.ok(box.x>=0&&box.y>=0&&box.x+box.width<=940&&box.y+box.height<=700,JSON.stringify(box))
    await page.screenshot({path:captures+'/menu-plain-text.png'})
  }finally{await browser.close();await server.close()}
})
