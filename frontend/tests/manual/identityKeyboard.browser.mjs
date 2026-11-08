// Optional real-browser behavioral test. Install Playwright separately or set
// PLAYWRIGHT_MODULE to its module path; CHROME_PATH selects local Chromium.
import fs from 'node:fs/promises'
import path from 'node:path'
import { createServer } from 'vite'
import os from 'node:os'
import {fileURLToPath} from 'node:url'
import { svelte } from '@sveltejs/vite-plugin-svelte'
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright')
import assert from 'node:assert/strict'
import postcss from 'postcss'
import tailwindcss from 'tailwindcss'
import loadConfig from 'tailwindcss/loadConfig.js'
const tailwindConfig = loadConfig(new URL('../../tailwind.config.js',import.meta.url).pathname)

const frontend = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const root = await fs.mkdtemp(path.join(os.tmpdir(), 'post-identity-browser-'))
await fs.mkdir(root, { recursive: true })
const original = await fs.readFile(process.env.IDENTITY_COMPONENT || path.join(frontend, 'src/lib/components/settings/account/AccountIdentityTab.svelte'), 'utf8')
let source = original.replaceAll("from '$lib/components/ui/button'", "from './ui.js'").replaceAll("from '$lib/components/ui/input'", "from './ui.js'").replaceAll("from '$lib/components/ui/label'", "from './ui.js'").replaceAll("from '$lib/components/ui/dialog'", "from './ui.js'")
source = source.replaceAll("from './IdentityEditor.svelte'", "from './Stub.svelte'").replaceAll("from '../AccountDialog.svelte'", "from './Stub.svelte'").replaceAll("from '@iconify/svelte'", "from './Stub.svelte'").replaceAll("from '$lib/stores/toast'", "from './mock.js'").replaceAll("from '$lib/stores/accounts.svelte'", "from './mock.js'").replaceAll("from '$lib/i18n'", "from './mock.js'").replaceAll("from '../../../../../wailsjs/go/models'", "from './models.js'").replaceAll("from '../../../../../wailsjs/go/app/App'", "from './mock.js'")
await fs.writeFile(path.join(root, 'Identity.svelte'), source)
const css = await postcss([tailwindcss({...tailwindConfig,content:[path.join(root,'Identity.svelte')]})]).process(await fs.readFile(path.join(frontend,'src/app.css'),'utf8'),{from:path.join(frontend,'src/app.css')})
await fs.writeFile(path.join(root,'app.css'),css.css)
await fs.copyFile(path.join(frontend,'src/themes.css'),path.join(root,'themes.css'))
await fs.cp(path.join(frontend,'src/themes'),path.join(root,'themes'),{recursive:true})
await fs.writeFile(path.join(root, 'Stub.svelte'), '<script>let {children,...props}=$props()</script><span {...props}>{@render children?.()}</span>')
await fs.writeFile(path.join(root, 'Button.svelte'), '<script>let {children,...props}=$props()</script><button {...props}>{@render children?.()}</button>')
await fs.writeFile(path.join(root, 'Dialog.svelte'), '<script>let {children,open=false}=$props()</script>{#if open}{@render children?.()}{/if}')
await fs.writeFile(path.join(root, 'ui.js'), "export {default as Button,default as Input,default as Label,default as Content,default as Header,default as Title,default as Description,default as Footer} from './Button.svelte';export {default as Root} from './Dialog.svelte'")
await fs.writeFile(path.join(root, 'models.js'), 'export const account = {}')
await fs.writeFile(path.join(root, 'mock.js'), `
import {writable} from 'svelte/store'
export const _ = writable((key) => ({'common.edit':'Edit','common.delete':'Delete','identity.defaultAddress':'Default address','identity.setAsDefaultAddress':'Set as default address','identity.emailAddresses':'Email addresses','identity.sharedMailboxes':'Shared mailboxes'}[key] || key))
const rows = ['a','b','c'].map((id,i)=>({id,email:id+'@example.invalid',name:'Identity '+id.toUpperCase(),isDefault:i===0,signatureEnabled:false}))
window.calls=[]; window.failNext=false
export async function GetIdentities(){return rows.map(x=>({...x}))}
export async function SetDefaultIdentity(account,id){window.calls.push([account,id]); if(window.failNext){window.failNext=false;throw Error('dummy failure')} await new Promise(r=>setTimeout(r,15)); for(const row of rows)row.isDefault=row.id===id}
export async function GetMicrosoftSharedMailboxes(){return [{id:'shared',email:'shared@example.invalid'}]}
export const addToast=()=>{}; export const accountStore={load:async()=>{},syncAccount:()=>{}}
export const CreateIdentity=async()=>{};export const UpdateIdentity=async()=>{};export const DeleteIdentity=async()=>{};export const AddMicrosoftSharedMailbox=async()=>{};export const RemoveAccount=async()=>{};
`)
await fs.writeFile(path.join(root, 'App.svelte'), `<script>import Identity from './Identity.svelte'</script><main style="padding:30px;max-width:720px"><h1>Identity settings keyboard check</h1><Identity accountId="account-one" editAccount={{authType:'oauth2',imapHost:'outlook.office365.com'}} /></main>`)
await fs.writeFile(path.join(root, 'index.html'), '<html><body><div id="app"></div><script type="module">import "./app.css";import {mount} from "svelte";import App from "./App.svelte";mount(App,{target:document.getElementById("app")})</script></body></html>')
await fs.symlink(path.join(frontend, 'node_modules'), path.join(root, 'node_modules')).catch(()=>{})
const server = await createServer({root,configFile:false, plugins:[svelte()],server:{host:'127.0.0.1',port:4193,fs:{allow:[root,await fs.realpath(path.join(frontend,'node_modules'))]}},optimizeDeps:{include:['svelte','svelte/store']}})
await server.listen()
const browser = await chromium.launch({executablePath:process.env.CHROME_PATH || '/usr/bin/google-chrome',headless:true,args:['--no-sandbox']})
try {
  const page = await browser.newPage({viewport:{width:1050,height:850}})
  page.on('pageerror', e=>console.log('PAGE ERROR',String(e)))
  await page.goto('http://127.0.0.1:4193')
  const radios=page.locator('input[type=radio]')
  await radios.first().waitFor()
  assert.equal(await radios.count(),3)
  await radios.nth(0).focus()
  await page.keyboard.press('ArrowDown')
  await page.waitForFunction(()=>window.calls.length===1)
  await page.waitForTimeout(80)
  assert.equal(await radios.nth(1).isChecked(),true)
  assert.equal(await radios.nth(1).evaluate(el=>el===document.activeElement),true,'focus must survive server success')
  await page.keyboard.press('ArrowRight')
  await page.waitForTimeout(80)
  assert.equal(await radios.nth(2).isChecked(),true)
  await page.keyboard.press('ArrowDown')
  await page.waitForTimeout(80)
  assert.equal(await radios.nth(0).isChecked(),true,'wrap from last to first')
  await page.keyboard.press('ArrowUp')
  await page.waitForTimeout(80)
  assert.equal(await radios.nth(2).isChecked(),true,'reverse wrap')
  await page.evaluate(()=>window.failNext=true)
  await page.keyboard.press('ArrowLeft')
  await page.waitForTimeout(80)
  assert.equal(await radios.nth(2).isChecked(),true,'backend failure restores current default')
  // Browser-managed Tab enters/leaves group without tabbing through all radios.
  await radios.nth(2).focus(); await page.keyboard.press('Tab')
  assert.equal(await page.evaluate(()=>document.activeElement.tagName),'BUTTON')
  // Verify each list's actual CSS focus disclosure independently.
  const actions = page.locator('div.opacity-0')
  assert.equal(await actions.count(),4)
  await page.mouse.move(1000,800)
  for (const index of [0,3]) {
    const wrapper=actions.nth(index)
    await wrapper.locator('button').first().focus()
    await page.waitForTimeout(200)
    assert.equal(await wrapper.evaluate(el=>getComputedStyle(el).opacity),'1')
  }
  console.log('PASS real Chromium: arrows, wrap, focus retained, backend failure rollback, native Tab, both list disclosures')
  await page.screenshot({path:process.env.IDENTITY_SCREENSHOT || path.join(root, 'identity-radio-check.png'),fullPage:true})
} finally {await browser.close();await server.close()}
