import {test} from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {realpathSync,mkdirSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const captureDir=process.env.POSTI_BROWSER_CAPTURE_DIR||join(tmpdir(),'posti-native-filter-captures')
mkdirSync(captureDir,{recursive:true})
test('native Gmail review: selected consent, preview invalidation, explicit publish, uncertain recovery and reopen',{skip:!browserModule,timeout:60000},async()=>{
  const root=fileURLToPath(new URL('..',import.meta.url))
  const server=await createServer({cacheDir:fileURLToPath(new URL(`../node_modules/.vite-test/${process.pid}/nativeGmail`,import.meta.url)),root,plugins:[{name:'native-fixture',resolveId(id){if(id==='/native-mount.js')return id},load(id){if(id==='/native-mount.js')return `export {mount,unmount} from 'svelte'`}}],server:{host:'127.0.0.1',port:5217,strictPort:true,fs:{allow:[root,realpathSync(join(root,'node_modules'))]}},logLevel:'error'})
  await server.listen()
  const {chromium}=await import(browserModule)
  const browser=await chromium.launch({executablePath:process.env.POSTI_CHROMIUM_BUNDLED?undefined:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  try{
    const page=await browser.newPage({viewport:{width:1024,height:1000}})
    const errors=[];page.on('pageerror',error=>errors.push(error.message))
    await page.route('**/native-fixture',route=>route.fulfill({contentType:'text/html',body:'<html data-theme="dark"><body style="background:#18191d"><div id="fixture"></div></body></html>'}))
    await page.goto('http://127.0.0.1:5217/native-fixture')
    await page.evaluate(async()=>{
      await import('/src/app.css'); await import('/src/lib/iconify-offline.ts');window.calls=[];window.consent=true;window.plan={};window.failCreate=true
      window.go={app:{App:new Proxy({},{get:(_,name)=>(...args)=>{
        window.calls.push([name,args])
        if(name==='GetNativeGmailWorkspace')return Promise.resolve({key:'helper-key',target:'helper',endpoint:'https://studio.example.test'})
        if(name==='AuthorizeNativeGmailWorkspace'){window.consent=false;return Promise.resolve()}
        if(name!=='NativeGmailReview')return Promise.reject(new Error('unexpected legacy local API: '+name))
        const req=args[0]
        if(req.workspace!=='helper-key')return Promise.reject(new Error('wrong workspace'))
        switch(req.operation){
          case 'status':return Promise.resolve({publication:window.plan})
          case 'context':return Promise.resolve({context:{accountId:'work',email:'me@example.com',consentRequired:window.consent,labels:[{id:'user-label',name:'Reading'}]}})
          case 'preview':return window.previewFails?Promise.reject(new Error('preview unavailable')):Promise.resolve({preview:{accountEmail:'reviewed@example.com',found:50,truncated:true,fingerprint:JSON.stringify(req.draft)}})
          case 'prepare':window.plan={id:'plan',ruleId:'source',accountId:'work',accountEmail:'reviewed@example.com',state:'prepared',fingerprint:req.fingerprint,query:req.draft.query,addLabelIds:req.draft.labelIds,removeLabelIds:['UNREAD']};return Promise.resolve({publication:window.plan})
          case 'publish':window.plan={...window.plan,state:window.rejectCreate?'rejected':'unknown'};return Promise.reject(new Error(window.rejectCreate?'Gmail rejected filter':'create timed out'))
          case 'reconcile':window.plan={...window.plan,state:'published',providerId:'gmail-id'};return Promise.resolve({publication:window.plan})
          case 'cancel':window.plan={};return Promise.resolve({})
          default:return Promise.reject(new Error('unexpected operation'))
        }
      }})}}
      const {mount,unmount}=await import('/native-mount.js')
      window.show=async(locale='en')=>{
        if(window.component)await unmount(window.component)
        await (await import('/src/lib/i18n/index.ts')).initI18n(locale)
        await new Promise(resolve=>setTimeout(resolve,100))
        const Component=(await import('/src/lib/components/common/NativeGmailFilterDialog.svelte')).default
        window.component=mount(Component,{target:document.querySelector('#fixture'),props:{open:true,ruleId:'source'}})
      }
      await window.show()
    })
    await page.getByRole('button',{name:'Authorize selected Gmail account'}).click()
    await page.getByText('Studio helper: https://studio.example.test',{exact:true}).waitFor()
    await page.getByLabel('Gmail search query').fill('from:newsletter@example.com')
    assert.deepEqual(await page.evaluate(()=>window.calls.find(([n])=>n==='AuthorizeNativeGmailWorkspace')[1]),['helper-key','work'])
    await page.getByLabel('Mark read',{exact:true}).check()
    await page.getByLabel('Reading',{exact:true}).check()
    const review=page.getByRole('button',{name:'Review exact Gmail rule'})
    assert.equal(await review.isDisabled(),true)
    await page.evaluate(()=>{window.previewFails=true})
    await page.getByRole('button',{name:'Preview existing mail'}).click()
    await page.getByRole('alert').filter({hasText:'preview unavailable'}).waitFor()
    assert.equal(await review.isDisabled(),true)
    await page.evaluate(()=>{window.previewFails=false})
    await page.getByRole('button',{name:'Preview existing mail'}).click()
    await page.getByText('50+ matching messages found (sample)',{exact:true}).waitFor()
    assert.equal(await review.isEnabled(),true)
    await page.screenshot({path:join(captureDir,'native-gmail-preview-en.png'),fullPage:true})
    await page.getByLabel('Gmail search query').fill('from:changed@example.com')
    assert.equal(await review.isDisabled(),true,'query changes invalidate preview')
    await page.getByRole('button',{name:'Preview existing mail'}).click()
    await review.click()
    await page.getByText('Final review: exact frozen Gmail rule',{exact:true}).waitFor()
    await page.getByText('reviewed@example.com',{exact:true}).waitFor()
    assert.equal(await page.evaluate(()=>window.calls.filter(([n,args])=>n==='NativeGmailReview'&&args[0].operation==='publish').length),0,'review never publishes')
    await page.getByRole('button',{name:'Cancel review and release source'}).click()
    await page.getByLabel('Gmail search query').waitFor()
    await page.getByRole('button',{name:'Preview existing mail'}).click()
    await review.click()
    await page.getByRole('button',{name:'Publish to Gmail',exact:true}).waitFor()
    await page.screenshot({path:join(captureDir,'native-gmail-final-review-en.png'),fullPage:true})
    await page.getByRole('button',{name:'Publish to Gmail',exact:true}).click()
    await page.getByRole('button',{name:'Check Gmail result'}).waitFor()
    assert.equal(await page.getByRole('button',{name:'Publish to Gmail',exact:true}).count(),0,'unknown result cannot create again')
    await page.screenshot({path:join(captureDir,'native-gmail-unknown-en.png'),fullPage:true})
    await page.evaluate(()=>{window.consent=true;return window.show()})
    await page.getByRole('button',{name:'Authorize selected Gmail account'}).waitFor()
    assert.equal(await page.getByRole('button',{name:'Check Gmail result'}).isDisabled(),true,'recovery waits for renewed consent')
    await page.setViewportSize({width:320,height:860})
    await page.screenshot({path:join(captureDir,'native-gmail-recovery-consent-en-320.png'),fullPage:true})
    await page.evaluate(()=>window.show('fi'))
    await page.getByText('from:changed@example.com',{exact:true}).waitFor()
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'renewed consent fits Finnish narrow view')
    await page.screenshot({path:join(captureDir,'native-gmail-recovery-consent-fi-320.png'),fullPage:true})
    await page.evaluate(()=>window.show('en'))
    await page.getByRole('button',{name:'Authorize selected Gmail account'}).click()
    await page.getByRole('button',{name:'Check Gmail result'}).waitFor()
    assert.equal(await page.getByRole('button',{name:'Check Gmail result'}).isEnabled(),true)
    assert.equal(await page.getByRole('button',{name:'Publish to Gmail',exact:true}).count(),0,'reopened unknown result cannot create again')
    await page.getByRole('button',{name:'Check Gmail result'}).click()
    await page.getByText('Gmail filter verified: gmail-id',{exact:true}).waitFor()
    assert.equal(await page.evaluate(()=>window.calls.filter(([n,args])=>n==='NativeGmailReview'&&args[0].operation==='publish').length),1)
    for(const locale of ['en','fi']){
      await page.setViewportSize({width:320,height:860})
      await page.evaluate(locale=>window.show(locale),locale)
      await page.waitForTimeout(500)
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'no horizontal overflow')
      await page.screenshot({path:join(captureDir,`native-gmail-published-${locale}-320.png`),fullPage:true})
    }

    // Exercise rejection in the already-open prepared review, not only reopen.
    await page.evaluate(()=>{window.plan={};window.rejectCreate=true;return window.show('en')})
    await page.getByLabel('Gmail search query').fill('from:rejected@example.com')
    await page.getByLabel('Mark read',{exact:true}).check()
    await page.getByRole('button',{name:'Preview existing mail'}).click()
    await page.getByRole('button',{name:'Review exact Gmail rule'}).click()
    await page.getByRole('button',{name:'Publish to Gmail',exact:true}).click()
    await page.getByText('Gmail rejected this filter without creating it.',{exact:false}).waitFor()
    assert.equal(await page.getByRole('button',{name:'Publish to Gmail',exact:true}).count(),0)
    assert.equal(await page.getByRole('button',{name:'Check Gmail result'}).count(),0)
    await page.screenshot({path:join(captureDir,'native-gmail-live-rejected-en-320.png'),fullPage:true})
    // Reopen must preserve the same safe cancellation-only state too.
    await page.evaluate(()=>window.show('en'))
    await page.getByText('Gmail rejected this filter without creating it.',{exact:false}).waitFor()
    await page.getByRole('button',{name:'Cancel review and release source'}).click()
    await page.getByLabel('Gmail search query').waitFor()
    assert.equal(await page.evaluate(()=>window.calls.filter(([n,args])=>n==='NativeGmailReview'&&args[0].operation==='publish').length),2)
    assert.deepEqual(errors,[])
  }finally{await browser.close();await server.close()}
})
