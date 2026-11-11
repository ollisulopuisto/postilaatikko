// Conversation (bubble) mode must render HTML email with the same fidelity as
// Traditional mode. It previously ran the body through normalizeConversationHtml
// (removing every <style>/<link> and stripping color/background/font inline
// properties) and injected a blanket `body * { ... !important }` rule, which
// flattened designed HTML email: LinkedIn headings shrank to body text, filled
// buttons turned into outlines and card backgrounds disappeared.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const body = await read('../src/lib/components/viewer/EmailBody.svelte')

test('conversation mode no longer strips authored email styles', () => {
  assert.doesNotMatch(body, /normalizeConversationHtml/)
  // The processed HTML is identical for compact and traditional rendering.
  assert.match(body, /const processedHtml = processHtml\(html, imagesBlocked\)/)
})

test('compact iframe CSS keeps email typography and element colors', () => {
  const compactBlock = body.slice(body.indexOf('${compact ?'), body.indexOf("` : ''}", body.indexOf('${compact ?')))
  assert.ok(compactBlock.length > 0, 'compact CSS block found')
  // No blanket strip of inline presentation in bubble mode.
  assert.doesNotMatch(compactBlock, /body \* \{/)
  assert.doesNotMatch(compactBlock, /font-size: inherit !important/)
  assert.doesNotMatch(compactBlock, /background-color: transparent !important/)
  // The bubble surface still shows through around the email itself.
  assert.match(compactBlock, /html, body \{ background: transparent !important/)
})
