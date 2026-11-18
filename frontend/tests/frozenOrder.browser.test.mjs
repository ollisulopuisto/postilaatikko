// Full App with deterministic Wails fixtures; never accesses a mailbox.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

test('frozen App index survives reading/selection and arrivals, explicit pill releases it in grouped and plain folders', { skip: !browserModule, timeout: 120000 }, async () => {
  const { chromium } = await import(browserModule)
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5193', '--strictPort'], { cwd: fileURLToPath(new URL('..', import.meta.url)), stdio: ['ignore', 'pipe', 'pipe'] })
  let log = ''; server.stdout.on('data', data => { log += data }); server.stderr.on('data', data => { log += data })
  let browser
  try {
    for (let i = 0; i < 150; i++) {
      try { if ((await fetch('http://127.0.0.1:5193')).ok) break } catch { /* starting */ }
      if (server.exitCode !== null || i === 149) throw new Error(log)
      await delay(100)
    }
    browser = await chromium.launch({ executablePath: process.env.POSTI_CHROMIUM_BUNDLED ? undefined : process.env.CHROME_BIN || '/usr/bin/google-chrome-stable', headless: true, args: ['--no-sandbox'] })
    for (const mode of ['categories', 'priority', 'plain']) {
      const plain = mode === 'plain'
      const page = await browser.newPage({ viewport: { width: 1360, height: 900 } })
      const errors = []; page.on('pageerror', err => errors.push(err.message))
      await page.addInitScript(({ plain, mode }) => {
        if (!plain) localStorage.setItem('eterno-mail:inbox-display', mode)
        const handlers = new Map()
        window.__emit = (name, data) => { for (const fn of handlers.get(name) || []) fn(data) }
        window.runtime = new Proxy({}, { get: (_, key) => key === 'EventsOnMultiple' ? (name, fn) => { handlers.set(name, [...handlers.get(name) || [], fn]); return () => {} } : () => Promise.resolve() })
        const make = (id, unread = 1) => ({ threadId: id, accountId: 'demo', folderId: 'inbox', subject: `Frozen fixture ${id}`, snippet: 'Keep this position while reading', latestDate: '2026-11-18T12:00:00Z', messageIds: [`msg-${id}`], messageCount: 1, unreadCount: unread, participants: [{ name: 'Fixture sender', email: 'fixture@gmail.com' }] })
        window.__rows = [make('one'), make('two'), make('three', 0)]
        window.__arrive = () => { window.__rows = [make('new'), ...window.__rows.slice().reverse()]; window.__emit('folder:synced', { accountId: 'demo', folderId: 'inbox' }) }
        const data = {
          IsReady: true, GetThemeMode: 'dark', GetLanguage: 'en', GetAccounts: [{ id: 'demo', name: 'Demo', email: 'demo@example.test', type: 'imap', enabled: true }],
          GetUIState: { selectedAccountId: plain ? 'demo' : 'unified', selectedFolderId: 'inbox', selectedFolderName: plain ? 'Fixture folder' : 'Inbox', selectedFolderType: plain ? 'folder' : 'inbox' },
          GetFolderTree: [{ folder: { id: 'inbox', accountId: 'demo', name: 'Fixture folder', type: plain ? 'folder' : 'inbox', path: 'INBOX' }, children: [] }],
          GetFolders: [{ id: 'inbox', accountId: 'demo', name: 'Fixture folder', type: plain ? 'folder' : 'inbox', children: [] }],
        }
        window.go = { app: { App: new Proxy({}, { get: (_, key) => (...args) => {
          if (key === 'GetUnifiedFolderConversations' || key === 'GetConversations') return Promise.resolve(structuredClone(window.__rows))
          if (key === 'GetUnifiedFolderCount' || key === 'GetConversationCount') return Promise.resolve(window.__rows.length)
          if (key === 'GetConversation') {
            const id = args[0]; const row = window.__rows.find(row => row.threadId === id) || window.__rows[0]
            return Promise.resolve({ id: row.threadId, subject: row.subject, messages: [{ id: `msg-${row.threadId}`, accountId: 'demo', folderId: 'inbox', threadId: row.threadId, subject: row.subject, fromName: 'Fixture sender', fromEmail: 'fixture@gmail.com', toList: [], ccList: [], date: row.latestDate, bodyText: `Body ${row.threadId}`, isRead: false }] })
          }
          if (key === 'MarkAsRead') { for (const row of window.__rows) if (args[0].includes(`msg-${row.threadId}`)) row.unreadCount = 0; window.__emit('messages:readChanged', { messageIds: args[0], isRead: true }) }
          return Promise.resolve(key in data ? data[key] : key.startsWith('Get') || key.startsWith('List') ? [] : null)
        } }) } }
      }, { plain, mode })
      await page.goto('http://127.0.0.1:5193', { waitUntil: 'domcontentloaded' })
      const rows = page.locator('[data-mail-row-key]'); await rows.first().waitFor()
      const identities = () => rows.evaluateAll(nodes => nodes.map(node => node.getAttribute('data-mail-row-key')))
      const original = await identities(); assert.equal(original.length, 3)
      const groupForOne = () => rows.filter({ hasText: 'Frozen fixture one' }).evaluate(node => node.closest('section')?.textContent || '')
      const groupBefore = await groupForOne()
      await rows.filter({ hasText: 'Frozen fixture one' }).click()
      await page.waitForTimeout(200)
      await page.evaluate(() => { window.__rows[0].unreadCount = 0; window.__emit('messages:readChanged', { messageIds: ['msg-one'], isRead: true }) })
      await rows.filter({ hasText: 'Frozen fixture two' }).click()
      await page.waitForTimeout(200)
      assert.deepEqual(await identities(), original, 'moving selection after read does not shift grouped or plain rows')
      if (!plain) assert.equal((await groupForOne()).includes('Frozen fixture three'), groupBefore.includes('Frozen fixture three'), 'read row stays in original card after another selection')
      await page.evaluate(() => window.__arrive()); await page.waitForTimeout(650)
      assert.deepEqual(await identities(), original, 'new mail/reordered backend stays outside list')
      const pill = page.getByRole('button', { name: /new or changed, tap to re-sort/ }); await pill.waitFor({ timeout: 5000 })
      if (process.env.POSTI_FROZEN_SCREENSHOT_DIR) {
        await page.screenshot({ path: `${process.env.POSTI_FROZEN_SCREENSHOT_DIR}/frozen-${mode}-dark.png` })
        await page.emulateMedia({ forcedColors: 'active' })
        await page.screenshot({ path: `${process.env.POSTI_FROZEN_SCREENSHOT_DIR}/frozen-${mode}-contrast.png` })
        await page.emulateMedia({ forcedColors: 'none' })
      }
      await pill.click(); await page.waitForTimeout(350)
      assert.equal((await identities()).length, 4)
      assert.notDeepEqual(await identities(), original)
      // A second passive arrival waits; starting a reply deliberately releases it.
      await page.evaluate(() => { window.__rows = [{ ...window.__rows[0], threadId: 'reply-new', subject: 'Reply release arrival', messageIds: ['msg-reply-new'] }, ...window.__rows]; window.__emit('folder:synced', { accountId: 'demo', folderId: 'inbox' }) })
      await pill.waitFor({ timeout: 5000 })
      assert.equal((await identities()).length, 4)
      await page.getByRole('button', { name: 'Reply', exact: true }).first().click()
      await page.waitForTimeout(250)
      assert.equal((await identities()).length, 5, 'reply releases pending membership')
      assert.equal(errors.length, 0, errors.join('\n'))
      await page.close()
    }
  } finally { await browser?.close(); server.kill(); await delay(150) }
})
