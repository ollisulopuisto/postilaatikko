// Optional actual CSS/store Chromium regression; backend and list data are fixtures.
import fs from 'node:fs/promises'
import assert from 'node:assert/strict'
import ts from 'typescript'
import os from 'node:os'
import path from 'node:path'
const outputDir = process.env.ANIMATION_SCREENSHOT_DIR || path.join(os.tmpdir(), 'post-animation-check')
await fs.mkdir(outputDir, {recursive:true})
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright')
const row = await fs.readFile(new URL('../../src/lib/components/list/ConversationRow.svelte', import.meta.url), 'utf8')
const style = row.slice(row.indexOf('  /* Operation exit animations'))
const css = style.slice(0, style.indexOf('</style>')).replaceAll(/:global\(([^)]+)\)/g, '$1').replaceAll('@keyframes -global-', '@keyframes ')
let store = await fs.readFile(new URL('../../src/lib/stores/messageAnimations.svelte.ts', import.meta.url), 'utf8')
store = ts.transpileModule(store.replaceAll('export ', ''), {compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText
const browser = await chromium.launch({executablePath:process.env.CHROME_PATH || '/usr/bin/google-chrome',headless:true,args:['--no-sandbox']})
try {
  const page = await browser.newPage({viewport:{width:900,height:500}})
  await page.setContent(`<style>:root{--muted:210 20% 90%;--destructive:0 70% 50%;--primary:220 70% 50%}body{font:18px sans-serif;margin:25px}.row{border:1px solid #ccc;padding:20px;height:80px;background:#fafafa;margin-bottom:8px}${css}</style><h2>Operation exit animation checks</h2><div id="cancel" class="row">Cancelled Trash deletion stays visible</div><div id="slow" class="row">Moved message awaits slow refresh</div><div id="other" class="row">Separate operation</div>`)
  await page.evaluate(code => { window.$state = x => x; window.store = new Function(code + ';return new MessageAnimationsStore()')() }, store)
  await page.screenshot({path:path.join(outputDir, 'post189-animations-before.png')})
  await page.evaluate(() => {
    window.store.startRemoval(['slow'], 'move')
    window.release = window.store.holdActive()
    window.store.settleRemoval(['slow'])
    document.querySelector('#slow').className='row row-anim-move'
  })
  await page.waitForTimeout(300)
  assert.equal(await page.locator('#cancel').evaluate(el=>getComputedStyle(el).opacity),'1')
  assert.equal(await page.locator('#slow').evaluate(el=>getComputedStyle(el).opacity),'0')
  await page.waitForTimeout(300)
  assert.equal(await page.locator('#slow').evaluate(el=>getComputedStyle(el).opacity),'0','no flash during slow reload')
  await page.screenshot({path:path.join(outputDir, 'post189-animations-held.png')})
  await page.evaluate(() => {document.querySelector('#slow').remove();window.release();window.store.startRemoval(['reader'], 'archive');window.store.settleRemoval(['reader'])})
  await page.waitForTimeout(260)
  assert.equal(await page.evaluate(()=>window.store.isAnimating('reader')),undefined)
  console.log('PASS rendered CSS/store: cancelled Trash visible, slow reload no flash, reader-only operation expires')
} finally {await browser.close()}
