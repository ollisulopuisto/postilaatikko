import { test } from 'node:test'
import assert from 'node:assert/strict'
import { replaceGeneratedBlocks, isUpToDate } from '../scripts/shortcuts-doc-blocks.mjs'

const mark = (name, body) =>
  `<!-- BEGIN GENERATED: ${name} (run \`npm run docs:shortcuts\` in frontend/; do not edit by hand) -->\n\n${body}\n\n<!-- END GENERATED: ${name} -->`
const doc = (a, b) => `# Title\n\nIntro prose.\n\n${mark('core', a)}\n\nMiddle prose.\n\n${mark('extensions', b)}\n\nTail prose.\n`

test('only text between the markers changes; the prose around it is kept', () => {
  const out = replaceGeneratedBlocks(doc('old a', 'old b'), { core: 'new a', extensions: 'new b' })
  assert.equal(out, doc('new a', 'new b'))
  assert.ok(out.includes('Intro prose.') && out.includes('Middle prose.') && out.includes('Tail prose.'))
})

test('a missing marker is an error, not a silent skip', () => {
  assert.throws(() => replaceGeneratedBlocks('# no markers\n', { core: 'x' }), /missing/)
})

test('up-to-date and stale docs are told apart', () => {
  const blocks = { core: 'a', extensions: 'b' }
  assert.equal(isUpToDate(doc('a', 'b'), blocks), true)
  assert.equal(isUpToDate(doc('stale', 'b'), blocks), false)
})

test('an up-to-date doc with CRLF line endings is not reported stale', () => {
  const blocks = { core: 'a', extensions: 'b' }
  assert.equal(isUpToDate(doc('a', 'b').replace(/\n/g, '\r\n'), blocks), true)
})
