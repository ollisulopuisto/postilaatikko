// P-035: which message and tone the account settings show for each Gmail API
// state. The mapping is pure so it can be checked without the component.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function load(relative) {
  const source = await readFile(new URL(relative, import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}

const { gmailApiStatusView } = await load('../src/lib/gmail/apiStatus.ts')

test('not applicable accounts show nothing', () => {
  assert.equal(gmailApiStatusView({ applicable: false, state: 'notUsed' }), null)
  assert.equal(gmailApiStatusView(undefined), null)
})

test('every state maps to a translation key and a tone', () => {
  const expected = {
    unknown: ['account.gmailApi.unknown', 'neutral'],
    active: ['account.gmailApi.active', 'good'],
    unavailable: ['account.gmailApi.unavailable', 'warning'],
    reauthRequired: ['account.gmailApi.reauthRequired', 'warning'],
    notUsed: ['account.gmailApi.notUsed', 'neutral'],
  }
  for (const [state, [key, tone]] of Object.entries(expected)) {
    const view = gmailApiStatusView({ applicable: true, state })
    assert.equal(view.key, key, state)
    assert.equal(view.tone, tone, state)
  }
})

test('an unrecognised state falls back to unknown rather than hiding the row', () => {
  const view = gmailApiStatusView({ applicable: true, state: 'something-new' })
  assert.equal(view.key, 'account.gmailApi.unknown')
})

test('since is passed through only when present', () => {
  assert.equal(gmailApiStatusView({ applicable: true, state: 'active', since: '2026-10-25T10:00:00Z' }).since, '2026-10-25T10:00:00Z')
  assert.equal(gmailApiStatusView({ applicable: true, state: 'active' }).since, undefined)
})
