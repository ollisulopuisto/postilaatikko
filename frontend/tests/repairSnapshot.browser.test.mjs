// Full App, real browser; only the Wails backend is a deterministic fixture.
// Opt in: POSTI_PLAYWRIGHT_PATH=/absolute/path/to/playwright/index.mjs node --test tests/repairSnapshot.browser.test.mjs
// No live mailbox or native app is used. The regular suite still covers layout policy.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

test('real App: offline repaired body uses snapshot content and live action identity', { skip: !browserModule, timeout: 60000 }, async () => {
  const { chromium } = await import(browserModule)
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5187', '--strictPort'], { cwd: fileURLToPath(new URL('..', import.meta.url)), stdio: ['ignore', 'pipe', 'pipe'] })
  let serverLog = ''
  server.stdout.on('data', data => { serverLog += data })
  server.stderr.on('data', data => { serverLog += data })
  let browser
  try {
    for (let i = 0; i < 100; i++) {
      try { if ((await fetch('http://127.0.0.1:5187')).ok) break } catch { /* Vite starting */ }
      if (server.exitCode !== null || i === 99) throw new Error(`Vite did not start: ${serverLog}`)
      await delay(100)
    }
    browser = await chromium.launch({ ...(process.env.POSTI_CHROMIUM_BUNDLED ? {} : { executablePath: process.env.CHROME_BIN || '/usr/bin/google-chrome-stable' }), headless: true, args: ['--no-sandbox'] })
    for (const width of [1280]) {
      const page = await browser.newPage({ viewport: { width, height: 800 } })
      const errors = []
      page.on('pageerror', err => errors.push(err.message))
      await page.addInitScript(() => {
        localStorage.setItem('postilaatikko:context-pane-auto-hide', 'true')
        window.runtime = new Proxy({}, { get: (_, key) => key === 'EventsOnMultiple' ? () => () => {} : () => Promise.resolve() })
        const data = {
          IsReady: true, GetThemeMode: 'dark', GetLanguage: 'en', GetAccounts: [{ id: 'demo', name: 'Demo', email: 'demo@example.test', type: 'imap', enabled: true }],
          GetUIState: { selectedAccountId: 'unified', selectedFolderId: 'inbox', selectedFolderName: 'Inbox', selectedFolderType: 'inbox' },
          GetUnifiedFolderCount: 1,
          GetUnifiedFolderConversations: [{ threadId: 'thread1', accountId: 'demo', folderId: 'inbox', subject: 'Message index regression', snippet: 'Click to open', latestDate: new Date().toISOString(), messageCount: 1, unreadCount: 1, participants: [{ name: 'Fixture sender', email: 'fixture@example.test' }] }],
          GetConversation: { id: 'thread1', subject: 'Message index regression', messages: [{ id: 'msg1', accountId: 'demo', folderId: 'inbox', threadId: 'thread1', subject: 'Message index regression', fromName: 'Fixture sender', fromEmail: 'fixture@example.test', toList: [], ccList: [], date: new Date().toISOString(), bodyText: 'Offline body from repaired snapshot.', bodyFetched: true, cachedBodySourceId: 'snapshot', hasAttachments:true, isRead: false }] },
        }
        window.calls=[]
        window.go = { app: { App: new Proxy({}, { get: (_, key) => (...args) => {window.calls.push({name:key,args});return Promise.resolve(key === 'ResolveMailActionTarget' ? args[0].messages.map(message => message.messageId) : key in data ? data[key] : key.startsWith('Get') || key.startsWith('List') ? [] : null)} }) } }
      })
      await page.goto('http://127.0.0.1:5187', { waitUntil: 'domcontentloaded' })
      await page.locator('[data-mail-row-key]').waitFor({state:'attached'})
      await page.mouse.move(25,300)
      await page.locator('[data-mail-row-key]').click()
      await page.getByText('Offline body from repaired snapshot.', {exact:true}).waitFor()
      await page.getByRole('button', {name: 'Mark as read', exact: true}).click()
      await page.waitForFunction(() => window.calls.some(c => c.name === 'MarkAsRead'))
      const calls = await page.evaluate(() => window.calls)
      assert.ok(calls.some(c => c.name === 'MarkAsRead' && JSON.stringify(c.args).includes('msg1')), 'a real read action must target live identity')
      assert.equal(calls.filter(c => c.name === 'FetchMessageBodyForAccount').length, 0, 'cached overlay must not fetch a body')
      assert.ok(calls.some(c => c.name === 'GetMessageAttachments' && c.args.includes('snapshot')), JSON.stringify(calls.filter(c => /Attachment/.test(c.name))))
      assert.equal(calls.filter(c => /MarkAsRead/.test(c.name)).some(c => JSON.stringify(c.args).includes('snapshot')), false, 'read action must retain live identity')
      if (process.env.POSTI_REPAIR_SCREENSHOT) await page.screenshot({path:process.env.POSTI_REPAIR_SCREENSHOT})
      assert.deepEqual(errors, [])
      await page.close()
    }
  } finally {
    if (browser) await browser.close()
    server.kill()
  }
})
