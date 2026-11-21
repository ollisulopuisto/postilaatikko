import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'

const read = relative => readFileSync(new URL(relative, import.meta.url), 'utf8')
test('parallel CI browser fixtures do not claim the same fixed port', () => {
  const workflow = read('../../.github/workflows/ci.yml')
  for (const batch of workflow.split('\n').filter(line => line.includes('node --test') && line.includes('.browser.test.mjs'))) {
    const seen = new Map()
    for (const file of batch.matchAll(/tests\/([\w.]+\.browser\.test\.mjs)/g)) {
      const source = read(`./${file[1]}`)
      for (const port of new Set([...source.matchAll(/port:\s*(\d+)/g)].map(match => match[1]))) {
        assert.equal(seen.has(port), false, `${file[1]} shares port ${port} with ${seen.get(port)}`)
        seen.set(port, file[1])
      }
    }
  }
})
test('Gmail cancellation guide includes prepared and definitive rejection, not ambiguous writes', () => {
  const guide = read('../../docs/GMAIL_FILTER_REVIEW.md')
  assert.ok(guide.includes('An unattempted prepared review or a definitively rejected create'))
  assert.ok(guide.includes('Ambiguous attempts and confirmed publications keep the local source frozen'))
  assert.equal(guide.includes('Cancel review releases only a prepared, never-attempted plan'), false)
})
