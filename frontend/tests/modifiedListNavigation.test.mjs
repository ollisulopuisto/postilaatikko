// Modified list navigation (Mod+Shift+] / Mod+Shift+[) and account switching
// (Mod+Alt+ArrowLeft/Right) — the shared-keymap chords. Covers three Devin
// review findings: shifted brackets produce }/{ on US layouts, the primary
// window's Ctrl/Cmd block must route the chords, and focusAdjacentAccount must
// read the rendered account list, and the account chords must open the
// all-folders panel first so the focused header is visible (Devin round 3).
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
  // Layouts that need AltGr/Option for the bracket still fire the chord
  assert.ok(KEY.LIST_NEXT(ev(']', { metaKey: true, shiftKey: true, altKey: true })))
  assert.ok(KEY.LIST_PREV(ev('[', { ctrlKey: true, shiftKey: true, altKey: true })))
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

test('the account chords move pane focus to the sidebar', () => {
  return readFile(path.join(frontendDir, 'src/App.svelte'), 'utf8').then(app => {
    const prev = app.match(/'nav\.accountPrev': [^\n]+/)
    const next = app.match(/'nav\.accountNext': [^\n]+/)
    assert.ok(prev && next, 'account chord handlers exist')
    assert.match(prev[0], /setFocusedPane\('sidebar'\)/, 'prev chord focuses the sidebar pane')
    assert.match(next[0], /setFocusedPane\('sidebar'\)/, 'next chord focuses the sidebar pane')
  })
})

test('focusAdjacentAccount skips accounts hidden by the folder quick filter', () => {
  return readFile(path.join(frontendDir, 'src/lib/components/sidebar/Sidebar.svelte'), 'utf8').then(sidebar => {
    const fn = sidebar.slice(sidebar.indexOf('export function focusAdjacentAccount'))
    const body = fn.slice(0, fn.indexOf('\n  }'))
    assert.match(body, /!listFilter \|\| trees\.length > 0/, 'hidden accounts are skipped, matching the template')
  })
})

test('focusAdjacentAccount reads the rendered account list', () => {
  return readFile(path.join(frontendDir, 'src/lib/components/sidebar/Sidebar.svelte'), 'utf8').then(sidebar => {
    const fn = sidebar.slice(sidebar.indexOf('export function focusAdjacentAccount'))
    const body = fn.slice(0, fn.indexOf('\n  }'))
    assert.match(body, /filteredAccountTrees/, 'account list comes from the rendered sections, not the folder-only nav list')
  })
})

test('focusAdjacentAccount opens the all-folders panel before focusing a header', () => {
  return readFile(path.join(frontendDir, 'src/lib/components/sidebar/Sidebar.svelte'), 'utf8').then(sidebar => {
    const fn = sidebar.slice(sidebar.indexOf('export function focusAdjacentAccount'))
    const body = fn.slice(0, fn.indexOf('\n  }'))
    assert.match(body, /showAllFolders = true/, 'a closed panel is opened so the focused header is visible')
    assert.match(body, /if \(collapsed\) onToggleCollapsed\?\.\(\)/, 'the compact rail expands first, like the More button')
    assert.ok(
      body.indexOf('showAllFolders = true') < body.indexOf('filteredAccountTrees'),
      'the panel opens before the account list is walked'
    )
  })
})

test('focusAdjacentAccount scrolls the header inside the panel, not the nav list', () => {
  return readFile(path.join(frontendDir, 'src/lib/components/sidebar/Sidebar.svelte'), 'utf8').then(sidebar => {
    const fn = sidebar.slice(sidebar.indexOf('export function focusAdjacentAccount'))
    const body = fn.slice(0, fn.indexOf('\n  }'))
    assert.match(body, /tick\(\)/, 'waits a tick so a just-opened panel has rendered the header')
    assert.match(body, /closest\('\.spark-sidebar'\)/, 'queries the sidebar root: account headers live in the panel, outside the nav scroll container')
  })
})
