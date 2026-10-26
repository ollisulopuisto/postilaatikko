// Deleting a virtual folder from the sidebar.
//
// window.confirm() does nothing in the macOS webview: Wails v2 does not
// implement the WKWebView JavaScript confirm panel, so the call returns false
// and the delete never happens. The sidebar must confirm through the app's own
// ConfirmDialog, and the row's delete control must stay reachable by keyboard
// (it was display:none until hovered).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const sidebar = await readFile(new URL('../src/lib/components/sidebar/Sidebar.svelte', import.meta.url), 'utf8')

// The sidebar row's delete button, from its aria-label to the closing tag.
function deleteButton(source) {
  const start = source.indexOf('aria-label={`Delete ${item.name}`}')
  assert.notEqual(start, -1, 'virtual folder delete button exists')
  const open = source.lastIndexOf('<button', start)
  return source.slice(open, source.indexOf('</button>', start))
}

test('deleting a virtual folder does not use window.confirm', () => {
  assert.doesNotMatch(sidebar, /window\.confirm\(/)
})

test('deleting a virtual folder confirms through the app ConfirmDialog', () => {
  assert.match(sidebar, /import ConfirmDialog from '\$lib\/components\/ui\/confirm-dialog\/ConfirmDialog\.svelte'/)
  assert.match(sidebar, /<ConfirmDialog[\s\S]*?variant="destructive"/)
})

test('the row delete button is reachable by keyboard, not only on hover', () => {
  const button = deleteButton(sidebar)
  assert.doesNotMatch(button, /\bhidden\b/, 'not display:none')
  assert.match(button, /focus-visible:opacity-100/)
})
