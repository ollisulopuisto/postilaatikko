// P-021 regression: the `v` / Alt+M move-to-folder shortcut must also work
// from the focused conversation viewer, not only the message list.
//
// This covers the stable action contract (`mail.moveTo`) that the viewer
// wiring in App.svelte relies on, plus the Finnish/AltGr-safe symbol
// predicate that lets `#` (Trash) and other symbols fire from layout keys
// without treating real Ctrl+Alt as a shortcut. Single-key shortcuts must
// stay disabled in editable fields (the `inInput` guard in App.svelte).

import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const frontendDir = fileURLToPath(new URL('..', import.meta.url))

const server = await createServer({
  root: frontendDir,
  configFile: path.join(frontendDir, 'vite.config.ts'),
  logLevel: 'error',
  server: { middlewareMode: true, hmr: false, watch: null },
  appType: 'custom',
  optimizeDeps: { noDiscovery: true, include: [] },
})
after(() => server.close())

const registry = await server.ssrLoadModule('/src/lib/keyboard/registry.ts')
const shortcutModule = await server.ssrLoadModule('/src/lib/keyboard/shortcuts.ts')
const { KEY } = shortcutModule
const { SHORTCUTS } = registry

function event(key, modifiers = {}) {
  return {
    key,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    getModifierState: () => false,
    ...modifiers,
  }
}

test('mail.moveTo is registered for the messageActions group with v and Alt+M', () => {
  const def = SHORTCUTS.find((d) => d.id === 'mail.moveTo')
  assert.ok(def, 'mail.moveTo shortcut is missing from the registry')
  assert.equal(def.group, 'messageActions')
  assert.deepEqual(def.combos, ['v', 'Alt+m'])
  assert.ok(!def.panes || def.panes.includes('mail'), 'mail.moveTo applies to the mail pane')
  assert.ok(def.predicates?.includes(KEY.LIST_MOVE_TO), 'mail.moveTo uses the LIST_MOVE_TO predicate')
})

test('LIST_MOVE_TO accepts bare v (move shortcut) and Alt+M', () => {
  assert.equal(KEY.LIST_MOVE_TO(event('v')), true, 'bare v triggers move')
  assert.equal(KEY.LIST_MOVE_TO(event('m', { altKey: true })), true, 'Alt+M triggers move')
  // Real modifier chords are not the move shortcut.
  assert.equal(KEY.LIST_MOVE_TO(event('m', { ctrlKey: true })), false, 'Ctrl+M is not move')
  assert.equal(KEY.LIST_MOVE_TO(event('m', { metaKey: true })), false, 'Cmd+M is not move')
  assert.equal(KEY.LIST_MOVE_TO(event('v', { altKey: true })), false, 'Alt+v is not move')
})

test('LAYOUT_SYMBOL accepts layout-produced # via Alt/AltGr but not real Ctrl+Alt', () => {
  // macOS Option+3 -> #
  assert.equal(shortcutModule.LAYOUT_SYMBOL(event('#', { altKey: true })), true)
  // Windows/Linux AltGr+3 -> # (reports Ctrl+Alt + AltGraph)
  assert.equal(
    shortcutModule.LAYOUT_SYMBOL(event('#', { altKey: true, ctrlKey: true, getModifierState: (k) => k === 'AltGraph' })),
    true,
    'AltGr+3',
  )
  // Actual Ctrl+Alt (no AltGraph) remains reserved, not a symbol shortcut.
  assert.equal(shortcutModule.LAYOUT_SYMBOL(event('#', { altKey: true, ctrlKey: true })), false)
  // Cmd combinations stay separate.
  assert.equal(shortcutModule.LAYOUT_SYMBOL(event('#', { metaKey: true, altKey: true })), false)
  // Alt+M is move, not a symbol.
  assert.equal(shortcutModule.LAYOUT_SYMBOL(event('m', { altKey: true })), false)
})
