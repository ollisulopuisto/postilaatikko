import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { realpathSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

// A reply-all to a large group must not push Send now, Undo or Edit recipients
// out of the viewport, at narrow and at short window sizes.
test('undo bar keeps its three controls on screen for a very large recipient list', { skip: !browserModule, timeout: 60000 }, async () => {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const server = await createServer({
    root,
    plugins: [{ name: 'toast-fixture', resolveId(id) { if (id === '/toast-mount.js') return id }, load(id) { if (id === '/toast-mount.js') return `export {mount} from 'svelte'` } }],
    server: { host: '127.0.0.1', port: 5194, strictPort: true, fs: { allow: [root, realpathSync(join(root, 'node_modules'))] } },
    logLevel: 'error',
  })
  await server.listen()
  const { chromium } = await import(browserModule)
  const browser = await chromium.launch({ executablePath: process.env.POSTI_CHROMIUM_BUNDLED ? undefined : '/usr/bin/google-chrome-stable', headless: true, args: ['--no-sandbox'] })
  try {
    const page = await browser.newPage({ viewport: { width: 1024, height: 700 } })
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/toast-fixture', r => r.fulfill({ contentType: 'text/html', body: '<html data-theme="dark"><body style="background:#18191d"><div id="fixture"></div></body></html>' }))
    await page.goto('http://127.0.0.1:5194/toast-fixture')
    await page.evaluate(async () => {
      await import('/src/app.css'); await import('/src/lib/iconify-offline.ts')
      await (await import('/src/lib/i18n/index.ts')).initI18n('en')
      const { mount } = await import('/toast-mount.js')
      const Toast = (await import('/src/lib/components/common/PendingSendToast.svelte')).default
      const many = n => Array.from({ length: n }, (_, i) => `Person Number ${i} <person.with.a.long.address.${i}@example-company.example>`).join(', ')
      mount(Toast, { target: document.querySelector('#fixture'), props: {
        dispatchAt: new Date(Date.now() + 20000).toISOString(), now: Date.now(),
        summary: { mode: 'reply-all', to: many(60), cc: many(60), bcc: many(30) },
        onSendNow() {}, onUndo() {}, onEditRecipients() {},
      } })
    })
    for (const [width, height] of [[1024, 700], [320, 480], [800, 260], [320, 240]]) {
      await page.setViewportSize({ width, height })
      await page.waitForTimeout(150)
      for (const name of ['Send now', 'Undo', 'Edit recipients']) {
        const box = await page.getByRole('button', { name, exact: true }).boundingBox()
        assert.ok(box, `${name} not rendered at ${width}x${height}`)
        assert.ok(box.x >= 0 && box.y >= 0 && box.x + box.width <= width && box.y + box.height <= height, `${name} outside ${width}x${height}: ${JSON.stringify(box)}`)
      }
      if (process.env.POSTI_TOAST_SHOT) await page.screenshot({ path: process.env.POSTI_TOAST_SHOT + width + 'x' + height + '.png' })
      const toast = await page.locator('[data-pending-send-toast]').boundingBox()
      assert.ok(toast.y >= 0 && toast.y + toast.height <= height, `toast taller than ${width}x${height}`)
    }
    assert.deepEqual(errors, [])
  } finally {
    await browser.close()
    await server.close()
  }
})
