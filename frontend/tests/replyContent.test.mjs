// UX-008: the pure decisions behind the conversation timeline: when a message
// body can be split into new text and quoted history, how that split is shown,
// and where the thread opens. The Svelte components only render these results.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

// Modules are transpiled and imported as data: URIs, so a relative import has
// to be rewritten to the data: URI of the module it names.
async function dataUri(relative, replacements = {}) {
  const source = await readFile(new URL(relative, import.meta.url), 'utf8')
  let { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  for (const [specifier, uri] of Object.entries(replacements)) {
    outputText = outputText.replaceAll(`'${specifier}'`, `'${uri}'`)
  }
  return `data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`
}

async function load(relative, replacements) {
  return import(await dataUri(relative, replacements))
}

const { canExtract, buildBodyPlan, textPreview, initialTimelineTarget, timelineScrollTop } = await load(
  '../src/lib/components/viewer/replyContent.ts',
)
const { restoreBlockedImages, hasBlockedRemoteImages } = await load('../src/lib/utils/blockedImages.ts')

const reply = (status, blocks, extra = {}) => ({
  state: 'cached',
  source: 'html',
  fromCache: false,
  result: { version: 'reply-v1', status, blocks, ...extra },
})
const block = (id, kind, content = `text ${id}`, format = 'html') => ({ id, kind, format, content })

test('only fetched, unencrypted messages with a body are extracted', () => {
  assert.equal(canExtract({ bodyFetched: true, bodyHtml: '<p>hi</p>' }), true)
  assert.equal(canExtract({ bodyFetched: true, bodyText: 'hi' }), true)
  assert.equal(canExtract({ bodyFetched: false, bodyHtml: '', bodyText: '' }), false, 'header only')
  assert.equal(canExtract({ bodyFetched: true, bodyHtml: '  ', bodyText: '\n' }), false, 'blank body')
  assert.equal(canExtract({ bodyFetched: true, bodyHtml: '<p>x</p>', hasSMIME: true }), false, 'S/MIME')
  assert.equal(canExtract({ bodyFetched: true, bodyText: 'x', hasPGP: true }), false, 'PGP')
})

test('new text stays visible and quoted history and signatures fold away', () => {
  const plan = buildBodyPlan(
    reply('extracted', [block('b1', 'authored'), block('b2', 'quoted'), block('b3', 'signature')]),
  )
  assert.equal(plan.mode, 'blocks')
  assert.deepEqual(
    plan.blocks.map((b) => [b.id, b.kind, b.collapsed]),
    [['b1', 'authored', false], ['b2', 'quoted', true], ['b3', 'signature', true]],
  )
})

test('text the extractor could not classify is never folded away', () => {
  const plan = buildBodyPlan(reply('partial', [block('b1', 'authored'), block('b2', 'unknown'), block('b3', 'quoted')]))
  assert.equal(plan.mode, 'blocks')
  assert.equal(plan.partial, true)
  assert.equal(plan.blocks.find((b) => b.id === 'b2').collapsed, false)
})

test('a block kind from a newer extractor is shown, not hidden', () => {
  const plan = buildBodyPlan(reply('partial', [block('b1', 'authored'), block('b2', 'heading')]))
  assert.equal(plan.mode, 'blocks')
  assert.equal(plan.blocks[1].collapsed, false)
})

test('inline replies keep the order of the original message', () => {
  const plan = buildBodyPlan(
    reply('extracted', [block('b1', 'quoted'), block('b2', 'authored'), block('b3', 'quoted'), block('b4', 'authored')]),
  )
  assert.deepEqual(plan.blocks.map((b) => b.id), ['b1', 'b2', 'b3', 'b4'])
  assert.deepEqual(plan.blocks.map((b) => b.collapsed), [true, false, true, false])
})

test('every uncertain or empty outcome falls back to the original message', () => {
  assert.deepEqual(buildBodyPlan(reply('fallback', [block('b1', 'unknown')])), { mode: 'original', reason: 'fallback' })
  assert.deepEqual(buildBodyPlan(reply('unavailable', [])), { mode: 'original', reason: 'unavailable' })
  assert.deepEqual(buildBodyPlan(null), { mode: 'original', reason: 'error' })
  assert.deepEqual(buildBodyPlan(undefined), { mode: 'original', reason: 'error' })
  assert.deepEqual(
    buildBodyPlan({ state: 'not_fetched', result: { status: 'unavailable', blocks: null } }),
    { mode: 'original', reason: 'unavailable' },
  )
})

test('a result that would show nothing falls back instead of showing a blank message', () => {
  const onlyFolded = buildBodyPlan(reply('extracted', [block('b1', 'quoted'), block('b2', 'signature')]))
  assert.deepEqual(onlyFolded, { mode: 'original', reason: 'fallback' })
  const blank = buildBodyPlan(reply('extracted', [block('b1', 'authored', '  \n ', 'text')]))
  assert.deepEqual(blank, { mode: 'original', reason: 'fallback' })
})

test('previews of folded blocks are short plain text', () => {
  assert.equal(textPreview('<div>On Mon, <b>Alice</b> &lt;a@example.org&gt; wrote:</div><p>Hi</p>', 'html', 200), 'On Mon, Alice <a@example.org> wrote: Hi')
  assert.equal(
    textPreview('<div>On Mon, Alice &lt;<a href="mailto:a@example.org">a@example.org</a>&gt; wrote:</div>', 'html', 200),
    'On Mon, Alice <a@example.org> wrote:',
  )
  assert.equal(textPreview('> line one\n>\n> line two', 'text', 200), '> line one > > line two')
  const long = textPreview('word '.repeat(100), 'text', 40)
  assert.ok(long.length <= 40 && long.endsWith('…'), long)
  assert.equal(textPreview('<style>p{}</style><script>evil()</script>x', 'html', 50), 'x')
})

test('the thread opens at the first unread message, else the newest', () => {
  const msgs = [
    { id: 'a', isRead: true },
    { id: 'b', isRead: false },
    { id: 'c', isRead: true },
    { id: 'd', isRead: false },
  ]
  assert.equal(initialTimelineTarget(msgs), 'b')
  assert.equal(initialTimelineTarget(msgs.map((m) => ({ ...m, isRead: true }))), 'd')
  assert.equal(initialTimelineTarget([]), null)
  assert.equal(initialTimelineTarget(null), null)
})

test('the target card keeps a peek of the message before it', () => {
  assert.equal(timelineScrollTop(500), 452)
  assert.equal(timelineScrollTop(20), 0)
  assert.equal(timelineScrollTop(500, 100), 400)
})

test('blocked remote images from the extractor can be restored on request', () => {
  const placeholder = `data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='100' height='100'%3E%3Ctext%3EImage blocked%3C/text%3E%3C/svg%3E`
  const blocked = `<p>Hi</p><img alt="logo" src="${placeholder}" data-original-src="https://cdn.example/a.png?x=1&amp;y=2" width="10"/>`
  assert.equal(hasBlockedRemoteImages(blocked), true)
  assert.equal(
    restoreBlockedImages(blocked),
    '<p>Hi</p><img alt="logo" src="https://cdn.example/a.png?x=1&amp;y=2" width="10"/>',
  )

  // Attribute order does not matter.
  const reordered = `<img data-original-src="http://t.example/p.gif" src="${placeholder}">`
  assert.equal(restoreBlockedImages(reordered), '<img src="http://t.example/p.gif">')
})

test('restoring never touches other images or non-web URLs', () => {
  const cid = '<img src="cid:logo@example.org">'
  const data = '<img src="data:image/png;base64,AAAA">'
  const odd = '<img src="data:image/svg+xml,x" data-original-src="javascript:alert(1)">'
  for (const html of [cid, data, odd]) {
    assert.equal(restoreBlockedImages(html), html)
  }
  assert.equal(hasBlockedRemoteImages(cid + data + odd), false)
  assert.equal(restoreBlockedImages(''), '')
})

// --- Opening a thread -------------------------------------------------------

const replyContentUri = await dataUri('../src/lib/components/viewer/replyContent.ts')
const { openTimelineAtTarget } = await load('../src/lib/components/viewer/timelineScroll.ts', {
  './replyContent': replyContentUri,
})

// A container whose card position can change, like a thread whose message
// bodies grow as they finish loading.
function fakeContainer(cards) {
  const listeners = new Map()
  const container = {
    scrollTop: 0,
    scrollHeight: 5000,
    cardTops: { ...cards },
    querySelector(selector) {
      const id = /data-message-id="([^"]+)"/.exec(selector)?.[1]
      return id && id in container.cardTops ? { id } : null
    },
    getBoundingClientRect: () => ({ top: 0 }),
    addEventListener(type, fn) {
      listeners.set(type, [...(listeners.get(type) ?? []), fn])
    },
    removeEventListener(type, fn) {
      listeners.set(type, (listeners.get(type) ?? []).filter((f) => f !== fn))
    },
    emit(type) {
      for (const fn of listeners.get(type) ?? []) fn()
    },
    listenerCount: () => [...listeners.values()].reduce((n, l) => n + l.length, 0),
  }
  // The card's top relative to the viewport depends on the scroll position.
  global.CSS = { escape: (s) => s }
  const original = container.querySelector
  container.querySelector = (selector) => {
    const card = original(selector)
    return card
      ? {
        getBoundingClientRect: () => {
          const top = container.cardTops[card.id] - container.scrollTop
          return { top, bottom: top + (container.cardHeights?.[card.id] ?? 400) }
        },
      }
      : null
  }
  return container
}

const thread = [
  { id: 'a', isRead: true },
  { id: 'b', isRead: false },
  { id: 'c', isRead: true },
]

test('opening a thread scrolls to the first unread card and keeps a peek of the previous one', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 1000, c: 1800 })
  openTimelineAtTarget(container, thread, () => true)
  assert.equal(container.scrollTop, 952)
})

test('the position is corrected while message bodies above it finish loading', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 1000, c: 1800 })
  openTimelineAtTarget(container, thread, () => true)
  container.cardTops.b = 1600 // the bodies above grew
  t.mock.timers.tick(1000)
  assert.equal(container.scrollTop, 1552)
})

test('the position is left alone once the reader scrolls', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 1000, c: 1800 })
  openTimelineAtTarget(container, thread, () => true)
  container.emit('wheel')
  container.scrollTop = 300
  container.cardTops.b = 1600
  t.mock.timers.tick(1000)
  assert.equal(container.scrollTop, 300)
})

test('a thread the reader already left is not scrolled', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 1000, c: 1800 })
  let current = true
  openTimelineAtTarget(container, thread, () => current)
  current = false
  container.scrollTop = 0
  t.mock.timers.tick(1000)
  assert.equal(container.scrollTop, 0)
})

test('listeners are removed after the settling period', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 1000, c: 1800 })
  openTimelineAtTarget(container, thread, () => true)
  assert.ok(container.listenerCount() > 0)
  t.mock.timers.tick(2000)
  assert.equal(container.listenerCount(), 0)
})

test('without a matching card the thread opens at the bottom', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({})
  openTimelineAtTarget(container, thread, () => true)
  assert.equal(container.scrollTop, 5000)
})

// --- Switching reader mode ---------------------------------------------------

const { captureScrollAnchor, holdScrollAnchor } = await load('../src/lib/components/viewer/timelineScroll.ts', {
  './replyContent': replyContentUri,
})
const { normalizeReaderMode, READER_MODES } = await load('../src/lib/components/viewer/readerMode.ts')

test('reader mode is conversation unless the reader chose traditional', () => {
  assert.deepEqual([...READER_MODES], ['conversation', 'traditional'])
  assert.equal(normalizeReaderMode('traditional'), 'traditional')
  assert.equal(normalizeReaderMode('conversation'), 'conversation')
  for (const unknown of [undefined, null, '', 'Traditional', 'compact', 7]) {
    assert.equal(normalizeReaderMode(unknown), 'conversation', String(unknown))
  }
})

test('the card at the top of the view is the anchor, with its offset', () => {
  const container = fakeContainer({ a: 0, b: 500, c: 1000 })
  container.cardHeights = { a: 500, b: 500, c: 500 }
  container.scrollTop = 620 // card b spans 500-1000, so it is cut at the top
  const anchor = captureScrollAnchor(container, ['a', 'b', 'c'])
  assert.deepEqual(anchor, { id: 'b', offset: -120 })
})

test('no anchor without a visible card', () => {
  assert.equal(captureScrollAnchor(fakeContainer({}), ['a']), null)
  assert.equal(captureScrollAnchor(fakeContainer({ a: 0 }), []), null)
})

test('the anchor card stays where it was while bodies around it change height', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 500, c: 1000 })
  container.scrollTop = 620
  const anchor = captureScrollAnchor(container, ['a', 'b', 'c'])
  container.cardTops.b = 900 // the message above grew after the mode switch
  holdScrollAnchor(container, anchor, () => true)
  assert.equal(container.cardTops.b - container.scrollTop, anchor.offset)
  container.cardTops.b = 1300 // and again as another body finished loading
  t.mock.timers.tick(1000)
  assert.equal(container.cardTops.b - container.scrollTop, anchor.offset)
})

test('the anchor is released when the reader scrolls', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const container = fakeContainer({ a: 0, b: 500 })
  const anchor = { id: 'b', offset: 0 }
  holdScrollAnchor(container, anchor, () => true)
  container.emit('keydown')
  container.scrollTop = 40
  container.cardTops.b = 900
  t.mock.timers.tick(1000)
  assert.equal(container.scrollTop, 40)
})
