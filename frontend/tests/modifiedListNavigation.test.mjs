// Modified list navigation (Mod+Shift+] / Mod+Shift+[) and account switching
// (Mod+Alt+ArrowLeft/Right) — the shared-keymap chords. Covers three Devin
// review findings: shifted brackets produce }/{ on US layouts, the primary
// window's Ctrl/Cmd block must route the chords, and focusAdjacentAccount must
// read the rendered account list.
import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createServer } from 'vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const frontendDir = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
const server = await createServer({ root: frontendDir, logLevel: 'silent', server: { middlewareMode: true } })
after(() => server.close())
const { KEY } = await server.ssrLoadModule('/src/lib/keyboard/shortcuts.ts')

const ev = (key, extra = {}) => ({ key, code: '', ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, getModifierState: () => false, ...extra })

test('LIST_NEXT and LIST_PREV accept the shifted bracket characters of US layouts', () => {
  // US keyboards produce } and { for Shift+] and Shift+[
  assert.ok(KEY.LIST_NEXT(ev('}', { ctrlKey: true, shiftKey: true })))
  assert.ok(KEY.LIST_NEXT(ev('}', { metaKey: true, shiftKey: true })))
  assert.ok(KEY.LIST_PREV(ev('{', { metaKey: true, shiftKey: true })))
  // The unshifted produced character works too (layouts where ] needs no Shift)
  assert.ok(KEY.LIST_NEXT(ev(']', { ctrlKey: true, shiftKey: true })))
  assert.ok(KEY.LIST_PREV(ev('[', { metaKey: true, shiftKey: true })))
  // No modifier, or the wrong one, must not fire
  assert.ok(!KEY.LIST_NEXT(ev(']', { shiftKey: true })))
  assert.ok(!KEY.LIST_NEXT(ev('}')))
  assert.ok(!KEY.LIST_PREV(ev('[')))
})

test('the primary window routes the modified list chords inside its Ctrl/Cmd block', () => {
  return readFile(path.join(frontendDir, 'src/App.svelte'), 'utf8').then(app => {
    const start = app.indexOf('// Handle Ctrl/Cmd shortcuts.')
    const end = app.indexOf('runRegistryShortcut(e)', start)
    assert.ok(start !== -1 && end !== -1, 'Ctrl/Cmd block found')
    const block = app.slice(start, end)
    assert.match(block, /KEY\.LIST_NEXT\(e\)/, 'Ctrl/Cmd block handles Mod+Shift+] before registry dispatch')
    assert.match(block, /KEY\.LIST_PREV\(e\)/, 'Ctrl/Cmd block handles Mod+Shift+[ before registry dispatch')
  })
})

test('focusAdjacentAccount reads the rendered account list', () => {
  return readFile(path.join(frontendDir, 'src/lib/components/sidebar/Sidebar.svelte'), 'utf8').then(sidebar => {
    const fn = sidebar.slice(sidebar.indexOf('export function focusAdjacentAccount'))
    const body = fn.slice(0, fn.indexOf('\n  }'))
    assert.match(body, /filteredAccountTrees/, 'account list comes from the rendered sections, not the folder-only nav list')
  })
})
