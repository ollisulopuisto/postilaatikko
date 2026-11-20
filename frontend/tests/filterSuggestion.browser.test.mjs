import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { realpathSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

// The backend event is Go JSON: criteria with no values arrive as null.
test('suggested filter dialog opens from a serialized suggestion with null criteria', { skip: !browserModule, timeout: 60000 }, async () => {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const server = await createServer({cacheDir:fileURLToPath(new URL(`../node_modules/.vite-test/${process.pid}/filterSuggestion`,import.meta.url)),
    root,
    plugins: [{ name: 'fixture', resolveId(id) { if (id === '/s-mount.js') return id }, load(id) { if (id === '/s-mount.js') return `export {mount} from 'svelte'` } }],
    server: { host: '127.0.0.1', port: 5195, strictPort: true, fs: { allow: [root, realpathSync(join(root, 'node_modules'))] } },
    logLevel: 'error',
  })
  await server.listen()
  const { chromium } = await import(browserModule)
  const browser = await chromium.launch({ executablePath: process.env.POSTI_CHROMIUM_BUNDLED ? undefined : '/usr/bin/google-chrome-stable', headless: true, args: ['--no-sandbox'] })
  try {
    const page = await browser.newPage({ viewport: { width: 1024, height: 1000 } })
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/s-fixture', r => r.fulfill({ contentType: 'text/html', body: '<html data-theme="dark"><body style="background:#18191d"><div id="fixture"></div></body></html>' }))
    await page.goto('http://127.0.0.1:5195/s-fixture')
    await page.evaluate(async () => {
      await import('/src/app.css'); await import('/src/lib/iconify-offline.ts')
      window.go = { app: { App: new Proxy({}, { get: (_, name) => () => {
        switch (name) {
          case 'GetAccounts': return Promise.resolve([{ id: 'a', name: 'Work', email: 'me@example.com' }])
          case 'PreviewFilterRule': return Promise.resolve(3)
          case 'GetFolderTree': return Promise.resolve([{ folder: { id: 'archive', path: 'Archive', name: 'Archive' }, children: [] }])
          default: return Promise.resolve([])
        }
      } }) } }
      await (await import('/src/lib/i18n/index.ts')).initI18n('en')
      const { mount } = await import('/s-mount.js')
      const Dialog = (await import('/src/lib/components/common/FilterDialog.svelte')).default
      const suggestion = JSON.parse('{"key":"k","accountId":"a","sourceFolderId":"inbox","destFolderId":"archive","destAccountId":"a","fromContains":null,"toContains":["team@lists.example"],"subjectContains":null}')
      mount(Dialog, { target: document.querySelector('#fixture'), props: { open: true, suggestion } })
    })
    await page.getByLabel('To', { exact: true }).first().waitFor()
    await page.waitForTimeout(600)
    assert.equal(await page.getByLabel('To', { exact: true }).first().isChecked(), true)
    assert.equal(await page.getByLabel('From', { exact: true }).first().isChecked(), false)
    assert.match(await page.locator('textarea, input[type=text]').evaluateAll(els => els.map(e => e.value).join('|')), /team@lists\.example/)
    assert.deepEqual(errors, [])
  } finally {
    await browser.close()
    await server.close()
  }
})
