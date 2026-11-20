import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { realpathSync, mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const browserModule = process.env.POSTI_PLAYWRIGHT_PATH

test('quit failure recovery text stays readable in English and Finnish at narrow widths', { skip: !browserModule, timeout: 60000 }, async () => {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const server = await createServer({cacheDir:fileURLToPath(new URL(`../node_modules/.vite-test/${process.pid}/quitFailure`,import.meta.url)),
    root,
    plugins: [{ name: 'quit-toast-fixture', resolveId(id) { if (id === '/quit-mount.js') return id }, load(id) { if (id === '/quit-mount.js') return `export {mount} from 'svelte'; export {get} from 'svelte/store'` } }],
    server: { host: '127.0.0.1', port: 5195, strictPort: true, fs: { allow: [root, realpathSync(join(root, 'node_modules'))] } },
    logLevel: 'error',
  })
  await server.listen()
  const { chromium } = await import(browserModule)
  const browser = await chromium.launch({ executablePath: process.env.POSTI_CHROMIUM_BUNDLED ? undefined : '/usr/bin/google-chrome-stable', headless: true, args: ['--no-sandbox'] })
  try {
    for (const lang of ['en', 'fi']) {
      const page = await browser.newPage({ viewport: { width: 390, height: 600 } })
      const errors = []
      page.on('pageerror', e => errors.push(e.message))
      await page.route('**/quit-fixture', r => r.fulfill({ contentType: 'text/html', body: '<html data-theme="dark"><body style="background:#18191d"><div id="fixture"></div></body></html>' }))
      await page.goto('http://127.0.0.1:5195/quit-fixture')
      await page.evaluate(async lang => {
        await import('/src/app.css'); await import('/src/lib/iconify-offline.ts')
        const i18n = await import('/src/lib/i18n/index.ts')
        await i18n.initI18n(lang)
        const { mount, get } = await import('/quit-mount.js')
        const Container = (await import('/src/lib/components/ui/toast/ToastContainer.svelte')).default
        mount(Container, { target: document.querySelector('#fixture') })
        const { addToast } = await import('/src/lib/stores/toast.ts')
        addToast({ type: 'error', message: get(i18n._)('pendingSend.quitFailed'), duration: 60000 })
      }, lang)
      for (const width of [320, 390, 1024]) {
        await page.setViewportSize({ width, height: 600 })
        const alert = page.getByRole('alert')
        await alert.waitFor(); await page.waitForTimeout(250)
        const box = await alert.boundingBox()
        assert.ok(box.x >= 0 && box.y >= 0 && box.x + box.width <= width && box.y + box.height <= 600)
        const clipped = await alert.locator('p').evaluate(p => p.scrollWidth > p.clientWidth || p.scrollHeight > p.clientHeight)
        assert.equal(clipped, false, `${lang} ${width}: recovery guidance must not be truncated`)
        if (process.env.POSTI_QUIT_SHOT_DIR) {
          mkdirSync(process.env.POSTI_QUIT_SHOT_DIR, { recursive: true })
          await page.screenshot({ path: join(process.env.POSTI_QUIT_SHOT_DIR, `quit-failure-${lang}-${width}.png`) })
        }
      }
      await page.setViewportSize({ width: 320, height: 240 })
      await page.evaluate(async () => {
        const { addToast } = await import('/src/lib/stores/toast.ts')
        addToast({ type: 'error', message: 'Long provider detail. '.repeat(100), duration: 60000 })
      })
      const stack = page.locator('.fixed.bottom-4')
      const stackBox = await stack.boundingBox()
      assert.ok(stackBox.y >= 0 && stackBox.y + stackBox.height <= 240, 'wrapped error stack stays inside a short window')
      await stack.hover(); await page.mouse.wheel(0, 1000); await page.waitForTimeout(150)
      assert.ok(await stack.evaluate(el => el.scrollTop > 0), 'long error details remain scrollable rather than covering the window')
      if (process.env.POSTI_QUIT_SHOT_DIR) await page.screenshot({ path: join(process.env.POSTI_QUIT_SHOT_DIR, `quit-error-scroll-${lang}.png`) })
      assert.deepEqual(errors, [])
      await page.close()
    }
  } finally { await browser.close(); await server.close() }
})
