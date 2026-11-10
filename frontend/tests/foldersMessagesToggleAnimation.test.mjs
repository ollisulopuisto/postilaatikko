// The Kansiot/Viestit switch must animate, not snap. Two layouts switch
// between folders and messages:
//  - narrow: the folder overlay slides over the list (transform transition).
//    It only ever animated if display:none did not defeat it — the overlay
//    now stays mounted and hides via transform + delayed visibility.
//  - wide shared column: the fold/unfold slides the column width
//    (.pane-shared), except while the divider is being dragged
//    (.pane-resizing keeps the pointer exact).
// Reduce Motion skips all of it.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
const css = await readFile(new URL('../src/app.css', import.meta.url), 'utf8')

test('narrow folder overlay stays mounted so the slide can play', () => {
  // display:none flips kill enter and leave transitions; narrow must keep
  // both panes mounted and let transform/visibility do the hiding.
  const displays = app.match(/style:display=\{contextPaneVisible[^\n]*/g) ?? []
  const panes = displays.filter(d => d.includes("contextPaneMode === 'folders'") || d.includes("contextPaneMode === 'messages'"))
  assert.equal(panes.length, 2, 'sidebar and list display rules found')
  for (const rule of panes) {
    assert.ok(rule.includes("getLayoutMode() === 'narrow'"), `narrow keeps pane mounted: ${rule}`)
  }
})

test('overlay hides with visibility delayed until the slide-out finishes', () => {
  assert.match(css, /\.responsive-sidebar-overlay \{[\s\S]*?visibility: hidden;[\s\S]*?transition: transform 0\.25s ease-out, visibility 0s linear 0\.25s;/)
  assert.match(css, /\.responsive-sidebar-visible \{[\s\S]*?visibility: visible;[\s\S]*?transition: transform 0\.25s ease-out, visibility 0s;/)
})

test('wide shared column slides width, except during divider drags', () => {
  assert.match(css, /\.pane-shared \{[\s\S]*?transition: width 0\.25s ease-out;/)
  assert.match(css, /\.pane-resizing \{\s*transition: none;\s*\}/)
  assert.ok((app.match(/class:pane-shared=\{sharedColumn\}/g) ?? []).length === 2, 'both panes carry pane-shared')
  assert.ok((app.match(/class:pane-resizing=\{isResizingList\}/g) ?? []).length === 2, 'both panes suppress the transition while dragging')
})

test('the covered message list leaves the accessibility tree in narrow folders view', () => {
  assert.match(app, /aria-hidden=\{getLayoutMode\(\) === 'narrow' && getResponsiveView\(\) === 'sidebar'/)
  assert.match(app, /inert=\{getLayoutMode\(\) === 'narrow' && getResponsiveView\(\) === 'sidebar'/)
})

test('Reduce Motion skips every toggle transition', () => {
  const reduce = css.match(/@media \(prefers-reduced-motion: reduce\) \{[\s\S]*?\n\}/)
  assert.ok(reduce, 'reduced-motion block exists')
  for (const cls of ['.responsive-sidebar-overlay', '.responsive-sidebar-visible', '.responsive-viewer-overlay', '.responsive-scrim', '.pane-shared']) {
    assert.ok(reduce[0].includes(cls), `reduced motion covers ${cls}`)
  }
})
