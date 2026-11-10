// Performance regression guard for EmailBody loading placeholder
// Continuous SMIL animations (<animateTransform>) inside iframes cause WebKit / Wails
// to constantly re-render and consume excessive (>100%) CPU cycles.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const emailBodyPath = path.resolve(__dirname, '../src/lib/components/viewer/EmailBody.svelte')

test('EmailBody.svelte does not contain continuous animateTransform SMIL animation in SVG placeholder', () => {
  const content = fs.readFileSync(emailBodyPath, 'utf-8')

  // Extract loadingPlaceholder string literal
  const match = content.match(/const loadingPlaceholder =\s*`([^`]+)`/)
  assert.ok(match, 'loadingPlaceholder definition must be present in EmailBody.svelte')

  const placeholderUri = match[1]

  // Must be an SVG data URI
  assert.ok(placeholderUri.startsWith('data:image/svg+xml,'), 'must start with data:image/svg+xml,')

  const decodedSvg = decodeURIComponent(placeholderUri.replace('data:image/svg+xml,', ''))

  // Must be well-formed SVG opening and closing
  assert.ok(decodedSvg.includes('<svg') && decodedSvg.includes('</svg>'), 'must be a valid SVG document')

  // Critical performance guard: must NOT contain <animateTransform> or <animate>
  assert.equal(
    /<animateTransform|<animate/i.test(decodedSvg),
    false,
    'placeholder SVG must not contain animateTransform or animate tags that cause continuous re-renders and >100% CPU usage'
  )
})

test('processCidReferences replaces cid references with placeholder and retains data-cid', () => {
  const content = fs.readFileSync(emailBodyPath, 'utf-8')
  const match = content.match(/const loadingPlaceholder =\s*`([^`]+)`/)
  assert.ok(match)
  const loadingPlaceholder = match[1]

  const CID_IMG_RE = /src=["']cid:([^"']+)["']/gi
  function processCidReferences(html) {
    if (!html) return html
    CID_IMG_RE.lastIndex = 0
    return html.replace(
      CID_IMG_RE,
      (match, contentId) => `src="${loadingPlaceholder}" data-cid="${contentId}"`
    )
  }

  const input = '<p>Photo:</p><img src="cid:img_12345" alt="test" />'
  const output = processCidReferences(input)
  assert.ok(output.includes(`src="${loadingPlaceholder}"`))
  assert.ok(output.includes('data-cid="img_12345"'))
  assert.equal(output.includes('src="cid:img_12345"'), false)
})

test('sendHeight in iframeScript guards against redundant height updates', () => {
  const content = fs.readFileSync(emailBodyPath, 'utf-8')

  assert.ok(content.includes('lastSentHeight'), 'iframeScript must track lastSentHeight')
  assert.ok(content.includes('requestAnimationFrame'), 'sendHeight must use requestAnimationFrame')
})
