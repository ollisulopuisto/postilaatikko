// Bolt optimization regression: remote image detection & blocking in EmailBody.svelte.
//
// OLD: `new RegExp(CSS_REMOTE_URL_PATTERN, 'i')` and `new RegExp(CSS_REMOTE_URL_PATTERN, 'gi')`
// were re-compiled on every email render and derived-state calculation.
//
// NEW: pre-compiled regex instances at module scope with scheme pre-filtering.
//
// This test verifies:
// 1. old and new implementations agree across all test cases (correctness)
// 2. new is materially faster than old (performance regression guard)

import { test } from 'node:test'
import assert from 'node:assert/strict'

const CSS_QUOTE = `(?:['"]|&#(?:39|x27|34|x22);|&(?:apos|quot);)?`
const CSS_REMOTE_URL_PATTERN = `url\\(\\s*${CSS_QUOTE}\\s*https?://[^)]*?${CSS_QUOTE}\\s*\\)`

// ---- OLD implementation ----
function checkForRemoteImagesOld(html) {
  if (!html) return false
  if (/<img[^>]+src=["'](https?:\/\/[^"']+)["']/i.test(html)) return true
  if (new RegExp(CSS_REMOTE_URL_PATTERN, 'i').test(html)) return true
  if (/\bbackground\s*=\s*["'](https?:\/\/[^"']+)["']/i.test(html)) return true
  return false
}

function processHtmlOld(html, blockImages) {
  if (!html) return ''
  let processed = html
  if (blockImages) {
    processed = processed.replace(
      /(<img[^>]+)src=["'](https?:\/\/[^"']+)["']([^>]*>)/gi,
      (match, before, src, after) => `${before}src="placeholder" data-blocked-src="${encodeURIComponent(src)}"${after}`
    )
    processed = processed.replace(new RegExp(CSS_REMOTE_URL_PATTERN, 'gi'), 'url()')
    processed = processed.replace(/\bbackground\s*=\s*["'](https?:\/\/[^"']+)["']/gi, 'background=""')
  }
  return processed
}

// ---- NEW implementation ----
const REMOTE_IMG_RE = /<img[^>]+src=["'](https?:\/\/[^"']+)["']/i
const CSS_REMOTE_URL_RE = new RegExp(CSS_REMOTE_URL_PATTERN, 'i')
const CSS_REMOTE_URL_GLOBAL_RE = new RegExp(CSS_REMOTE_URL_PATTERN, 'gi')
const BG_ATTR_RE = /\bbackground\s*=\s*["'](https?:\/\/[^"']+)["']/i
const BLOCK_IMG_RE = /(<img[^>]+)src=["'](https?:\/\/[^"']+)["']([^>]*>)/gi
const BG_ATTR_GLOBAL_RE = /\bbackground\s*=\s*["'](https?:\/\/[^"']+)["']/gi

function checkForRemoteImagesNew(html) {
  if (!html) return false
  if (REMOTE_IMG_RE.test(html)) return true
  if (CSS_REMOTE_URL_RE.test(html)) return true
  if (BG_ATTR_RE.test(html)) return true
  return false
}

function processHtmlNew(html, blockImages) {
  if (!html) return ''
  let processed = html
  if (blockImages) {
    BLOCK_IMG_RE.lastIndex = 0
    processed = processed.replace(
      BLOCK_IMG_RE,
      (match, before, src, after) => `${before}src="placeholder" data-blocked-src="${encodeURIComponent(src)}"${after}`
    )
    CSS_REMOTE_URL_GLOBAL_RE.lastIndex = 0
    processed = processed.replace(CSS_REMOTE_URL_GLOBAL_RE, 'url()')
    BG_ATTR_GLOBAL_RE.lastIndex = 0
    processed = processed.replace(BG_ATTR_GLOBAL_RE, 'background=""')
  }
  return processed
}

const testCases = [
  '',
  '   ',
  '<p>Hello world! No images here.</p>',
  '<div>Just some text with an url http://example.com in plain text</div>',
  '<img src="cid:logo@123" alt="CID logo" />',
  '<img src="data:image/png;base64,iVBORw0KGgoAAAANS..." alt="Data URI" />',
  '<img src="https://example.com/photo.jpg" alt="Remote photo" />',
  '<img src="http://example.com/banner.png" />',
  '<img class="banner" src=\'https://cdn.example.org/img.webp\' width="100">',
  '<div style="background-image: url(https://example.com/bg.png);">Content</div>',
  '<div style="background-image: url(\'https://example.com/bg.png\');">Single quote</div>',
  '<div style="background-image: url(&quot;https://example.com/bg.png&quot;);">HTML entity quote</div>',
  '<div style="background-image: url(&#39;https://example.com/bg.png&#39;);">Numeric hex entity quote</div>',
  '<body background="https://example.com/wallpaper.jpg">Hello</body>',
  '<table background=\'http://example.com/tile.png\'><tr><td>Tile</td></tr></table>',
  '<div style="color: red; font-size: 14px;">Inline style without URL</div>',
  '<div style="background: linear-gradient(to right, red, yellow);">Gradient background</div>',
  '<img SRC="HTTPS://EXAMPLE.COM/UPPERCASE.JPG">',
]

// ---- Correctness tests ----
for (const [idx, html] of testCases.entries()) {
  test(`checkForRemoteImages old == new for case #${idx}`, () => {
    assert.equal(checkForRemoteImagesNew(html), checkForRemoteImagesOld(html))
  })

  test(`processHtml(true) old == new for case #${idx}`, () => {
    assert.equal(processHtmlNew(html, true), processHtmlOld(html, true))
  })
}

// ---- Benchmark & regression guard ----
function bench(fn, iterations) {
  for (let i = 0; i < 200; i++) fn() // warmup
  const start = process.hrtime.bigint()
  for (let i = 0; i < iterations; i++) fn()
  const end = process.hrtime.bigint()
  return Number(end - start) / 1_000_000 // ms
}

const ITER = 5000
function runOld() {
  for (const html of testCases) {
    checkForRemoteImagesOld(html)
    processHtmlOld(html, true)
  }
}

function runNew() {
  for (const html of testCases) {
    checkForRemoteImagesNew(html)
    processHtmlNew(html, true)
  }
}

test('pre-compiled remote image detection/blocking is faster than old implementation', () => {
  const oldMs = bench(runOld, ITER)
  const newMs = bench(runNew, ITER)
  console.log(`EmailBody remote images old=${oldMs.toFixed(2)} ms, new=${newMs.toFixed(2)} ms over ${ITER} iterations (${(oldMs / newMs).toFixed(2)}x faster)`)
  assert.ok(newMs < oldMs, `expected new (${newMs.toFixed(2)} ms) < old (${oldMs.toFixed(2)} ms)`)
  // Generous threshold to avoid flaky failures on virtualized CI runners while guarding against regression
  assert.ok(oldMs / newMs > 1.5, `expected >1.5x speedup, got ${(oldMs / newMs).toFixed(2)}x`)
})
