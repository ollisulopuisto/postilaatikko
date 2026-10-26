// UX-002: the TypeScript side of the view, thread and message-reference
// contract. internal/views/model_test.go checks the same fixtures from Go, so a
// field added on one side only fails here or there.

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

async function fixture(name) {
  return JSON.parse(await readFile(new URL(`../../internal/views/testdata/${name}`, import.meta.url), 'utf8'))
}

const {
  isThreadView,
  isThreadPage,
  scopeError,
  messageKey,
  threadKey,
  matchedRefs,
  completenessOf,
} = await load('../src/lib/views/guards.ts')

const threadFixtures = ['thread_duplicate_copies.json', 'thread_partial_bodies.json']

test('the shared thread fixtures satisfy the TypeScript contract', async () => {
  for (const name of threadFixtures) {
    assert.equal(isThreadView(await fixture(name)), true, name)
  }
  assert.equal(isThreadPage(await fixture('page_all_inboxes_partial.json')), true)
})

test('the guards reject unknown fields, missing fields and unknown states', async () => {
  const view = await fixture('thread_partial_bodies.json')

  assert.equal(isThreadView({ ...view, extra: 1 }), false, 'unknown top-level field')
  const withoutSubject = { ...view }
  delete withoutSubject.subject
  assert.equal(isThreadView(withoutSubject), false, 'missing subject')

  const badState = structuredClone(view)
  badState.messages[0].contentState = 'downloaded'
  assert.equal(isThreadView(badState), false, 'unknown contentState')

  const unqualified = structuredClone(view)
  delete unqualified.messages[0].ref.accountId
  assert.equal(isThreadView(unqualified), false, 'a message ref without an account')

  const page = await fixture('page_all_inboxes_partial.json')
  assert.equal(isThreadPage({ ...page, items: 'none' }), false, 'items must be an array')
})

test('derived fields agree with the messages', async () => {
  for (const name of threadFixtures) {
    const view = await fixture(name)
    assert.equal(view.messageCount, view.messages.length, name)
    assert.equal(view.matchedCount, matchedRefs(view.messages).length, name)
    assert.deepEqual(completenessOf(view.messages), view.completeness, name)

    const dates = view.messages.map((m) => Date.parse(m.date))
    assert.deepEqual([...dates].sort((a, b) => a - b), dates, `${name} is chronological`)
    assert.equal(Date.parse(view.latestDate), dates[dates.length - 1], name)
  }
})

test('copies with one RFC Message-ID keep distinct, account-qualified refs', async () => {
  const view = await fixture('thread_duplicate_copies.json')
  assert.equal(view.messages[0].rfcMessageId, view.messages[1].rfcMessageId)
  const keys = new Set(view.messages.map((m) => messageKey(m.ref)))
  assert.equal(keys.size, view.messages.length)
  assert.deepEqual(matchedRefs(view.messages), [view.messages[0].ref])
})

test('threads from two accounts stay separate on a page', async () => {
  const page = await fixture('page_all_inboxes_partial.json')
  assert.notEqual(threadKey(page.items[0].ref), threadKey(page.items[1].ref))
  assert.equal(page.complete, false)
  assert.equal(page.errors[0].accountId, 'acc-c')
})

test('keys do not collide when an id contains the separator', () => {
  assert.notEqual(
    messageKey({ accountId: 'a/b', messageId: 'c' }),
    messageKey({ accountId: 'a', messageId: 'b/c' }),
  )
  assert.notEqual(
    messageKey({ accountId: 'acc-a', messageId: 'm-1' }),
    messageKey({ accountId: 'acc-b', messageId: 'm-1' }),
  )
})

test('completenessOf counts body states like the Go implementation', () => {
  const m = (contentState) => ({ contentState })
  assert.deepEqual(completenessOf([]), { complete: true, bodies: { total: 0, cached: 0, notFetched: 0, fetchFailed: 0 } })
  assert.deepEqual(completenessOf([m('cached'), m('cached')]).complete, true)
  assert.deepEqual(completenessOf([m('cached'), m('not_fetched')]), {
    complete: false,
    bodies: { total: 2, cached: 1, notFetched: 1, fetchFailed: 0 },
  })
  assert.deepEqual(completenessOf([m('fetch_failed'), m('cached')]), {
    complete: false,
    bodies: { total: 2, cached: 1, notFetched: 0, fetchFailed: 1 },
  })
})

test('scopeError accepts and rejects the same scopes as the Go Validate', async () => {
  const scopes = await fixture('scopes.json')
  scopes.valid.forEach((scope, i) => assert.equal(scopeError(scope), null, `valid[${i}]`))
  scopes.invalid.forEach((scope, i) => assert.notEqual(scopeError(scope), null, `invalid[${i}]`))
})
