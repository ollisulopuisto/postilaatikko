import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function load() {
  const source = await readFile(new URL('../src/lib/scheduledRepeat.ts', import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}

const { applyListMode, buildRepeat, defaultRepeatForm, NO_REPEAT, parseRepeatSummary, repeatError } = await load()
const person = (address, name = '') => ({ name, address })
const first = new Date(2026, 10, 16, 9, 0, 0)

test('a disabled repeat form sends the empty rule, never null', () => {
  assert.deepEqual(buildRepeat(defaultRepeatForm(), 'Europe/Helsinki'), NO_REPEAT)
  assert.equal(repeatError(defaultRepeatForm(), first), '')
})

test('an enabled form builds a monthly rule with its limits', () => {
  const form = { ...defaultRepeatForm(), enabled: true, every: 1, ends: 'count', count: 6 }
  assert.deepEqual(buildRepeat(form, 'Europe/Helsinki'), { unit: 'month', every: 1, zone: 'Europe/Helsinki', count: 6 })
  const until = { ...form, ends: 'until', until: '2027-03-01' }
  const rule = buildRepeat(until, 'Europe/Helsinki')
  assert.equal(new Date(rule.until).getFullYear(), 2027)
  assert.equal(rule.count, undefined)
})

test('invalid repeat input is named, not sent', () => {
  const on = { ...defaultRepeatForm(), enabled: true }
  assert.equal(repeatError({ ...on, every: 0 }, first), 'repeatEveryInvalid')
  assert.equal(repeatError({ ...on, every: 1.5 }, first), 'repeatEveryInvalid')
  assert.equal(repeatError({ ...on, ends: 'count', count: 1 }, first), 'repeatCountInvalid')
  assert.equal(repeatError({ ...on, ends: 'until', until: '' }, first), 'repeatUntilInvalid')
  assert.equal(repeatError({ ...on, ends: 'until', until: '2026-11-15' }, first), 'repeatUntilInvalid')
  assert.equal(repeatError({ ...on, ends: 'until', until: '2026-11-16' }, first), '')
})

test('repeat summaries round-trip and reject junk', () => {
  assert.deepEqual(parseRepeatSummary('month:2'), { unit: 'month', every: 2 })
  assert.equal(parseRepeatSummary('year:1'), null)
  assert.equal(parseRepeatSummary(undefined), null)
})

test('list mode puts the sender in To and everyone else in Bcc once', () => {
  const me = person('me@example.com', 'Me')
  const message = { subject: 'Pay', to: [person('A@example.com')], cc: [person('b@example.com')], bcc: [person('a@example.com'), person('ME@example.com'), person('')] }
  const out = applyListMode(message, 'bcc', me)
  assert.deepEqual(out.to, [me])
  assert.deepEqual(out.bcc.map(p => p.address), ['A@example.com', 'b@example.com'])
  assert.deepEqual(out.cc, [])
  assert.equal(out.subject, 'Pay')
})

test('list mode Cc keeps recipients visible and off mode changes nothing', () => {
  const me = person('me@example.com')
  const message = { to: [person('a@example.com')], cc: [], bcc: [person('c@example.com')] }
  const out = applyListMode(message, 'cc', me)
  assert.deepEqual(out.cc.map(p => p.address), ['a@example.com', 'c@example.com'])
  assert.deepEqual(out.bcc, [])
  assert.equal(applyListMode(message, 'off', me), message)
})
