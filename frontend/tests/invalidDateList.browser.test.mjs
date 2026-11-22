import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { fileURLToPath } from 'node:url'
import { realpathSync } from 'node:fs'
import { join } from 'node:path'

const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

// Regression: one conversation with an invalid latestDate (a half-synced old
// message with a missing/unparsable Date header) used to throw
// "Invalid time value" from formatRelativeDate inside the row render. On the
// initial list render that killed the whole message list - zero rows mounted;
// on a sync reload it broke the render mid-pass and left inconsistent DOM.
// The list must render every row and never throw for a bad date.
test('message list survives a conversation with an invalid latestDate', { skip: !browserModule, timeout: 90000 }, async () => {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const server = await createServer({
    root,
    cacheDir: join(root, 'node_modules/.vite-test', String(process.pid), 'invalidDateList'),
    plugins: [{ name: 'fixture-mount', resolveId(id) { if (id === '/fixture-mount.js') return id }, load(id) { if (id === '/fixture-mount.js') return `export {mount} from 'svelte'` } }],
    server: { host: '127.0.0.1', port: 5292, strictPort: true, fs: { allow: [root, realpathSync(join(root, 'node_modules'))] } },
    logLevel: 'error',
  })
  await server.listen()
  const { chromium } = await import(browserModule)
  const browser = await chromium.launch({ executablePath: process.env.POSTI_CHROMIUM_BUNDLED ? undefined : '/usr/bin/google-chrome-stable', headless: true, args: ['--no-sandbox'] })
  try {
    const page = await browser.newPage({ viewport: { width: 420, height: 900 } })
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/invalid-date-fixture', r => r.fulfill({ contentType: 'text/html', body: '<html class="dark" data-theme="dark"><body style="margin:0"><div id="fixture"></div></body></html>' }))
    await page.goto('http://127.0.0.1:5292/invalid-date-fixture')
    await page.evaluate(async () => {
      await import('/src/app.css'); await import('/src/lib/iconify-offline.ts')
      localStorage.setItem('eterno-mail:inbox-display', 'chronological')
      window.calls = []; window.listeners = new Map()
      window.runtime = new Proxy({}, { get: (_, key) => key === 'EventsOnMultiple'
        ? (name, callback) => { const list = window.listeners.get(name) || []; list.push(callback); window.listeners.set(name, list); return () => {} }
        : () => Promise.resolve() })
      const day = 86400000, now = new Date('2026-11-22T18:00:00Z').getTime()
      window.CONVS = Array.from({ length: 60 }, (_, i) => ({
        threadId: 't' + String(i).padStart(3, '0'), accountId: 'demo', folderId: 'inbox-a',
        subject: 'Synthetic subject ' + i, snippet: 'Synthetic snippet ' + i,
        // t003 models a half-synced old message whose Date header is missing.
        latestDate: i === 3 ? '' : new Date(now - (i < 2 ? 1 : i < 8 ? 5 + i : 10 + i) * day).toISOString(),
        messageCount: 1, unreadCount: i < 6 ? 1 : 0,
        participants: [{ name: 'Sender ' + i, email: 'sender' + i + '@example.test' }],
        messageIds: ['m' + i],
      }))
      window.go = { app: { App: new Proxy({}, { get: (_, name) => async (...args) => {
        window.calls.push([name, args])
        if (name === 'GetConversations') { const [, , offset = 0, limit = 50] = args; return window.CONVS.slice(offset, offset + limit) }
        if (name === 'GetConversationCount') return window.CONVS.length
        if (name === 'GetFTSIndexStatus') return { isComplete: true, totalCount: 1, indexedCount: 1 }
        if (name === 'IsFTSIndexing') return false
        return []
      } }) } }
      await (await import('/src/lib/i18n/index.ts')).initI18n('fi')
      const { accountStore } = await import('/src/lib/stores/accounts.svelte.ts')
      accountStore.accounts = [{ account: { id: 'demo', name: 'Demo', email: 'demo@example.test', enabled: true, authType: 'imap', imapHost: 'imap.example.test' }, folders: [], loading: false, syncing: false, error: null, lastSync: null }]
      const { mount } = await import('/fixture-mount.js')
      const C = (await import('/tests/fixtures/BlankRowsWorkspace.svelte')).default
      mount(C, { target: document.querySelector('#fixture') })
    })

    // Initial render: the bad date must not kill the list.
    await page.locator('[data-mail-row-key]').first().waitFor({ timeout: 30000 })
    await page.waitForTimeout(600)
    const initial = await page.locator('[data-mail-row-key]').count()
    assert.equal(initial, 50, `expected the first page of 50 rows, got ${initial}`)
    const corruptRow = await page.locator('[data-mail-row-key*="t003"]').count()
    assert.equal(corruptRow, 1, 'the conversation with the invalid date still renders')
    const groups = await page.locator('.inbox-display-group').count()
    assert.ok(groups >= 2, `chronological day groups render (got ${groups})`)

    // Sync reload path: corrupt another row and reload; the list re-renders whole.
    await page.evaluate(() => {
      window.CONVS = window.CONVS.map(c => c.threadId === 't010' ? { ...c, latestDate: 'not-a-date' } : c)
      for (const cb of window.listeners.get('messages:updated') || []) cb({ accountId: 'demo', folderId: 'inbox-a' })
    })
    await page.waitForTimeout(1200)
    const afterReload = await page.locator('[data-mail-row-key]').count()
    assert.equal(afterReload, 50, `rows survive a sync reload with a bad date, got ${afterReload}`)

    assert.deepEqual(errors.filter(e => /Invalid time value/.test(e)), [], `date render errors: ${errors.join(' | ')}`)
  } finally {
    await browser.close(); await server.close()
  }
})
