import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { writeFile, rm } from 'node:fs/promises'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

test('draft settings disclose attachment reads and upgrade legacy reply authorization', { skip: !browserModule, timeout: 60000 }, async () => {
  const { chromium } = await import(browserModule)
  const root = fileURLToPath(new URL('..', import.meta.url))
  const harness = new URL('../.draft-access-test.html', import.meta.url)
  await writeFile(harness, '<html class="dark"><body><main style="max-width:760px;padding:24px;margin:auto"></main><script type="module">import "/src/app.css";import {mount} from "svelte";import {initI18n} from "/src/lib/i18n/index.ts";import View from "/src/lib/components/settings/MailboxServiceAccess.svelte";await initI18n();mount(View,{target:document.querySelector("main")});</script></body></html>')
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5188', '--strictPort'], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] })
  let log = ''
  server.stdout.on('data', data => { log += data })
  server.stderr.on('data', data => { log += data })
  let browser
  try {
    for (let i = 0; i < 100; i++) {
      try { if ((await fetch('http://127.0.0.1:5188/.draft-access-test.html')).ok) break } catch { /* starting */ }
      if (server.exitCode !== null || i === 99) throw new Error(log)
      await delay(100)
    }
    browser = await chromium.launch({ ...(process.env.POSTI_CHROMIUM_BUNDLED ? {} : { executablePath: process.env.CHROME_BIN || '/usr/bin/google-chrome-stable' }), headless: true, args: ['--no-sandbox'] })
    const page = await browser.newPage({ viewport: { width: 1050, height: 1100 } })
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.addInitScript(() => {
      window.calls = []
      const status = { connected: true, writeAuthorized: true, accountManagementAuthorized: true, draftAccessAuthorized: false, sendAuthorized: true }
      window.go = { app: { App: new Proxy({}, { get: (_, key) => async () => {
        window.calls.push(key)
        if (key === 'GetLanguage') return 'en'
        if (key === 'GetMailboxServiceEndpoint') return 'http://127.0.0.1:8029'
        if (key === 'GetMailboxServiceLifecycleStatus') return { supported: false }
        if (key === 'EnableMailboxServiceDraftAccess') { status.draftAccessAuthorized = true; return { ...status } }
        if (key === 'GetMailboxServiceStatus') return { ...status }
        return null
      } }) } }
    })
    await page.goto('http://127.0.0.1:5188/.draft-access-test.html')
    const button = page.getByRole('button', { name: 'Enable draft and reply access', exact: true })
    await button.waitFor()
    assert.ok(await page.getByText(/read original attachments to prepare replies and forwards/).isVisible())
    if (process.env.POSTI_DRAFT_SCREENSHOT) await page.screenshot({ path: process.env.POSTI_DRAFT_SCREENSHOT, fullPage: true })
    await button.click()
    await page.getByText('Draft and reply preparation access, including original attachment reads, is enabled for this desktop.', { exact: true }).waitFor()
    assert.equal(await page.evaluate(() => window.calls.filter(name => name === 'EnableMailboxServiceDraftAccess').length), 1)
    if (process.env.POSTI_DRAFT_SCREENSHOT) await page.screenshot({ path: process.env.POSTI_DRAFT_SCREENSHOT.replace('.png', '-enabled.png'), fullPage: true })
    assert.deepEqual(errors, [])
  } finally {
    if (browser) await browser.close()
    server.kill()
    await rm(harness, { force: true })
  }
})
