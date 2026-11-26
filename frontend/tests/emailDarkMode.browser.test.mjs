/* global adaptEmailDarkMode */
import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync,mkdirSync} from 'node:fs'
import ts from 'typescript'
const browserModule=process.env.POSTI_PLAYWRIGHT_PATH
const source=readFileSync(new URL('../src/lib/utils/emailDarkMode.ts',import.meta.url),'utf8')
const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.None,target:ts.ScriptTarget.ES2022}}).outputText.replace('export function','function')
test('D1 authored CSS or adaptive colors, photos unchanged and light untouched',{skip:!browserModule},async()=>{
  const {chromium}=await import(browserModule)
  const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome-stable',headless:true,args:['--no-sandbox']})
  const captures=process.env.POSTI_BROWSER_CAPTURE_DIR||'/tmp/posti-d1-captures';mkdirSync(captures,{recursive:true})
  try{
    const page=await browser.newPage({viewport:{width:800,height:500},colorScheme:'dark'})
    const photo='data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="240" height="100"><rect width="240" height="100" fill="#fa8128"/><circle cx="60" cy="50" r="35" fill="#2878ce"/></svg>')
    const html=`<html><head><style>body{font:18px system-ui;padding:24px;background:white;color:#111}.card{background:#fafafa;padding:18px}a{color:#2563eb}</style></head><body><h1>D1 email rendering</h1><div class="card"><h2>Newsletter</h2><p>Readable text and <a href="https://example.invalid">a link</a>.</p><img src="${photo}"></div></body></html>`
    await page.setContent(html);await page.locator('img').evaluate(img=>img.decode())
    await page.locator('img').screenshot()
    await page.addScriptTag({content:code})
    assert.equal(await page.evaluate(()=>adaptEmailDarkMode(document)),'adapted')
    assert.equal(await page.locator('body').evaluate(n=>getComputedStyle(n).backgroundColor),'rgb(32, 36, 43)')
    assert.equal(await page.locator('p').evaluate(n=>getComputedStyle(n).color),'rgb(228, 231, 236)')
    assert.equal(await page.locator('img').evaluate(n=>getComputedStyle(n).filter),'none')
    assert.equal(await page.locator('html').evaluate(n=>getComputedStyle(n).filter),'none')
    assert.equal(await page.locator('img').getAttribute('src'),photo)
    await page.screenshot({path:`${captures}/d1-adapted.png`})
    await page.setContent(html.replace('</style>','@media (prefers-color-scheme: dark){body,.card{background:#152b35;color:#c9f5e8}a{color:#ffce70}}</style>'))
    await page.addScriptTag({content:code});assert.equal(await page.evaluate(()=>adaptEmailDarkMode(document)),'authored')
    assert.equal(await page.locator('body').evaluate(n=>getComputedStyle(n).backgroundColor),'rgb(21, 43, 53)')
    assert.equal(await page.locator('a').evaluate(n=>getComputedStyle(n).color),'rgb(255, 206, 112)')
    await page.screenshot({path:`${captures}/d1-authored.png`})
    await page.setContent('<body style="background:white;color:#111"><section style="background:linear-gradient(90deg,#fff7cd,#d6eaff)"><h1>Light hero, dark headline</h1></section></body>')
    await page.addScriptTag({content:code});await page.evaluate(()=>adaptEmailDarkMode(document))
    assert.equal(await page.locator('h1').evaluate(n=>getComputedStyle(n).color),'rgb(17, 17, 17)')
    await page.screenshot({path:`${captures}/d1-hero.png`})
    await page.emulateMedia({colorScheme:'light'});await page.setContent(html)
    assert.equal(await page.locator('body').evaluate(n=>getComputedStyle(n).backgroundColor),'rgb(255, 255, 255)')
  }finally{await browser.close()}
})
