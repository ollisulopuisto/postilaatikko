// Fails when two actions claim the same key in the same context.
//
// Context = rail pane + registry group. Groups are focus contexts (sidebar,
// messageList, viewer, composer, ...), so the same key in different focus
// groups can be fine. Every such overlap must be listed in BY_DESIGN with the
// reason the focus context decides it. A new overlap that is not listed fails
// the test, and so does a stale entry. Two actions in the same group with the
// same key are never allowed unless listed too.

import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { createServer } from 'vite'

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

const { CORE_SHORTCUTS, EXTENSION_REGISTRIES } = await server.ssrLoadModule('/src/lib/keyboard/registry.ts')

const BY_DESIGN = {
  'mail|Mod+z': 'composer open: the global handler returns early, the composer owns undo',
  'mail|Mod+w': 'composer open: the global handler returns early, the composer owns close',
  'mail|Escape': 'composer open: the composer owns Escape; otherwise the list does',
  'mail|Mod+u': 'composer open: underline; otherwise read later',
  'mail|Mod+i': 'composer open: italic; otherwise reply later',
  'mail|ArrowDown': 'the focused pane (sidebar, list, viewer) decides what moves',
  'mail|j': 'the focused pane (sidebar, list, viewer) decides what moves',
  'mail|ArrowUp': 'the focused pane (sidebar, list, viewer) decides what moves',
  'mail|k': 'the focused pane (sidebar, list, viewer) decides what moves',
  'mail|Enter': 'sidebar focus toggles the item, list focus opens the message',
  'mail|Space': 'sidebar focus toggles the item, list focus ticks the checkbox',
  'mail|Mod+a': 'viewer focus selects text, list focus selects messages',
  'mail|Shift+f': 'one key: message focus when a message is focused, else thread focus',
}

function collect(defs, pane) {
  const map = new Map()
  for (const d of defs) {
    for (const c of d.combos) {
      for (const p of (d.panes ?? [pane])) {
        const key = `${p}|${c}`
        map.set(key, [...(map.get(key) ?? []), d])
      }
    }
  }
  return map
}

const registries = [['mail', CORE_SHORTCUTS], ...EXTENSION_REGISTRIES.map(r => [r.id ?? r.extensionId, r.shortcuts])]

test('no duplicate binding in one context', () => {
  const seen = new Set()
  const unexplained = []
  for (const [pane, defs] of registries) {
    for (const [key, list] of collect(defs, pane)) {
      if (list.length < 2) continue
      seen.add(key)
      if (!(key in BY_DESIGN)) unexplained.push(`${key}: ${list.map(d => `${d.group}:${d.id}`).join(', ')}`)
    }
  }
  assert.deepEqual(unexplained, [], 'unexplained duplicate bindings (fix them or add to BY_DESIGN with a reason)')
  const stale = Object.keys(BY_DESIGN).filter(k => !seen.has(k))
  assert.deepEqual(stale, [], 'BY_DESIGN entries that no longer collide')
})

test('same-group duplicates are only the documented Shift+f toggle', () => {
  const bad = []
  for (const [pane, defs] of registries) {
    for (const [key, list] of collect(defs, pane)) {
      const groups = new Map()
      for (const d of list) groups.set(d.group, [...(groups.get(d.group) ?? []), d.id])
      for (const [g, ids] of groups) if (ids.length > 1 && key !== 'mail|Shift+f') bad.push(`${key} in ${g}: ${ids.join(', ')}`)
    }
  }
  assert.deepEqual(bad, [])
})

test('Cmd/Ctrl+K belongs to the palette only; E is archive and mark read', () => {
  const k = CORE_SHORTCUTS.filter(d => d.combos.includes('Mod+k')).map(d => d.id)
  assert.deepEqual(k, ['app.commandPalette'])
  const e = CORE_SHORTCUTS.filter(d => d.combos.includes('e')).map(d => d.id)
  assert.deepEqual(e, ['mail.archiveAndMarkRead'])
})
