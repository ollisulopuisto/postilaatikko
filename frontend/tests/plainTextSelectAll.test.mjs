import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'

const current = fs.readFileSync(new URL('../src/lib/components/viewer/EmailBody.svelte', import.meta.url), 'utf8')
const source = process.env.SELECT_ALL_SOURCE ? fs.readFileSync(process.env.SELECT_ALL_SOURCE, 'utf8') : current
const body = source.match(/function selectAllInIframe\(\) \{([\s\S]*?)\n {2}\}/)[1]
const select = new Function('bodyHtml', 'iframeElement', 'plainTextElement', 'document', 'window', `let ctxMenuVisible = true; ${body}; return ctxMenuVisible`)

test('plain-text Select all selects the whole rendered body and closes menu', () => {
  const message = {textContent: 'first line\nhttps://example.com\nlast line'}
  let selected, cleared = false
  const range = {selectNodeContents(node) { selected = node }}
  const selection = {removeAllRanges() { cleared = true }, addRange(value) { assert.equal(value, range) }}
  const closed = select('', null, message, {createRange: () => range}, {getSelection: () => selection})
  assert.equal(selected, message)
  assert.equal(cleared, true)
  assert.equal(closed, false)
  assert.match(current, /bind:this=\{plainTextElement\}/)
})

test('HTML Select all retains iframe message routing and closes menu', () => {
  let sent
  const iframe = {contentWindow: {postMessage(...args) { sent = args }}}
  const closed = select('<p>HTML</p>', iframe, null, {}, {})
  assert.deepEqual(sent, [{type: 'select-all'}, '*'])
  assert.equal(closed, false)
})
