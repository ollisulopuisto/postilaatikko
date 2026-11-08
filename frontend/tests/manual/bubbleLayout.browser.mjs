// Optional Chromium layout regression, using the viewer's actual row classes and CSS.
// PLAYWRIGHT_MODULE can point at an independently installed Playwright package.
import fs from 'node:fs/promises'
import assert from 'node:assert/strict'
import os from 'node:os'
import path from 'node:path'
const outputDir = process.env.LAYOUT_SCREENSHOT_DIR || path.join(os.tmpdir(), 'post-bubble-layout')
await fs.mkdir(outputDir, { recursive: true })
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright')
const source = await fs.readFile(new URL('../../src/lib/components/viewer/ConversationViewer.svelte', import.meta.url), 'utf8')
const row = source.match(/class=\{readerMode === 'conversation' \? `([^`]+)`/)[1]
const style = source.slice(source.lastIndexOf('<style>') + 7, source.lastIndexOf('</style>'))
const css = style.slice(0, style.indexOf('  .viewer-toolbar-action'))
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || '/usr/bin/google-chrome', headless: true, args: ['--no-sandbox'] })
try {
  const page = await browser.newPage()
  for (const width of [1000, 375]) {
    await page.setViewportSize({ width, height: 650 })
    const className = side => row.replace(/\$\{bubble\?\.side === 'right' \? '([^']+)' : '([^']+)'\}/, (_, right, left) => side === 'right' ? right : left)
    const mail = '<table width="600"><tr><td>HTML table mail fixture</td></tr></table>'
    const card = side => `<section class="${className(side)}"><div class="conversation-bubble-avatar">${side === 'right' ? 'Me' : 'A'}</div><div class="conversation-bubble bubble-${side}"><iframe title="${side}" srcdoc="${mail.replaceAll('"', '&quot;')}"></iframe></div></section>`
    await page.setContent(`<style>section{margin:12px 0}body{margin:20px;font:16px sans-serif;background:#fafafa}*{box-sizing:border-box}.flex{display:flex}.items-end{align-items:flex-end}.gap-2\\.5{gap:10px}.justify-start{justify-content:flex-start}.justify-end{justify-content:flex-end}.flex-row-reverse{flex-direction:row-reverse}.conversation-bubble-avatar{flex-shrink:0;width:40px;height:40px;margin-bottom:4px;border-radius:100%;display:grid;place-items:center;background:#2463eb;color:white}.conversation-bubble{padding:16px;border:1px solid #aaa; margin:0}iframe{display:block;width:100%;height:90px;border:0}${css}</style><h2>HTML mail, incoming and outgoing</h2>${card('left')}${card('right')}`)
    const boxes = await page.locator('section').evaluateAll(rows => rows.map(r => {
      const avatar = r.querySelector('.conversation-bubble-avatar').getBoundingClientRect()
      const bubble = r.querySelector('.conversation-bubble').getBoundingClientRect()
      const frame = r.querySelector('iframe').getBoundingClientRect()
      const tail = getComputedStyle(r.querySelector('.conversation-bubble'), '::after')
      return { avatar: avatar.toJSON(), bubble: bubble.toJSON(), frame: frame.toJSON(), tail: { left: tail.left, right: tail.right, bottom: tail.bottom, size: tail.width } }
    }))
    assert.ok(boxes[0].avatar.right <= boxes[0].bubble.left)
    assert.ok(boxes[1].avatar.left >= boxes[1].bubble.right)
    assert.ok(boxes[1].bubble.right > width * .75, 'outgoing bubble sits at right edge')
    assert.equal(boxes[0].tail.left, '-9px')
    assert.equal(boxes[1].tail.right, '-9px')
    for (const box of boxes) {
      assert.ok(box.bubble.right <= width - 19)
      assert.ok(box.bubble.left >= 19)
      assert.ok(box.frame.width > (width > 800 ? 600 : 240), 'HTML iframe keeps available reader width')
      assert.ok(Math.abs(box.avatar.bottom - (box.bubble.bottom - 24)) < 30, 'avatar aligns near bubble tail')
    }
    await page.screenshot({ path: path.join(outputDir, `post192-html-${width}.png`) })
    console.log(`PASS ${width}px: avatar/bubble/tail geometry and HTML iframe width`)
  }
} finally { await browser.close() }
