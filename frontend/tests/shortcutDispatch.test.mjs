// Every registry entry that is dispatched (dispatch: true) must resolve from
// its own combos through resolveShortcut(), in every focus pane, and no other
// entry may take its key. This is what stops a key being bound twice.

import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { readFile } from 'node:fs/promises'
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

const { CORE_SHORTCUTS } = await server.ssrLoadModule('/src/lib/keyboard/registry.ts')
const { resolveShortcut, matchCombo } = await server.ssrLoadModule('/src/lib/keyboard/dispatch.ts')

const NAMED = { Space: ' ' }
function eventFor(combo, mod = 'ctrl') {
  const parts = combo.split('+')
  const key = parts.pop()
  const k = NAMED[key] ?? key
  const shift = parts.includes('Shift')
  return {
    key: shift && k.length === 1 ? k.toUpperCase() : k,
    ctrlKey: parts.includes('Ctrl') || (parts.includes('Mod') && mod === 'ctrl'),
    metaKey: parts.includes('Meta') || (parts.includes('Mod') && mod === 'meta'),
    altKey: parts.includes('Alt'),
    shiftKey: shift,
  }
}

const dispatched = CORE_SHORTCUTS.filter(d => d.dispatch)
const ids = dispatched.map(d => d.id)
const FOCUS = ['sidebar', 'messageList', 'viewer']

test('every dispatched combo resolves to its own entry in every focus pane', () => {
  for (const def of dispatched) {
    for (const combo of def.combos) {
      if (def.legacy?.includes(combo)) continue
      for (const focus of FOCUS) {
        if (def.group === 'viewer' && focus !== 'viewer') continue
        for (const mod of ['ctrl', 'meta']) {
          const hit = resolveShortcut(eventFor(combo, mod), focus, ids)
          // Same combo in two groups: the focused pane's group wins (Mod+a).
          const rivals = dispatched.filter(d => d.id !== def.id && d.combos.includes(combo) && !d.legacy?.includes(combo))
          const winner = rivals.find(d => d.group === focus) ? rivals.find(d => d.group === focus) : def
          if (rivals.length && !rivals.some(d => d.group === focus) && def.group !== focus && def.group === 'viewer') continue
          assert.equal(hit?.id, winner.id, `${combo} in ${focus} (${mod}) resolves to ${hit?.id}, expected ${winner.id}`)
        }
      }
    }
  }
})

test('keys the registry does not bind resolve to nothing', () => {
  for (const key of ['k', 'K', 'q', 'w', 'p', 'b', 'i', 'y', 'n', 'h']) {
    const hit = resolveShortcut({ key, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false }, 'messageList', ids)
    if (hit) assert.ok(hit.combos.includes(key), `${key} -> ${hit.id}`)
  }
  // Cmd/Ctrl+K belongs to the palette (predicate), never to a dispatched entry.
  for (const mod of ['ctrl', 'meta']) {
    assert.equal(resolveShortcut(eventFor('Mod+k', mod), 'messageList', ids), undefined)
  }
  assert.equal(matchCombo({ key: 'e', ctrlKey: true, metaKey: false, altKey: false, shiftKey: false }, 'e'), false)
})

test('App.svelte has no per-key switch for dispatched commands', async () => {
  const src = await readFile(path.join(frontendDir, 'src/App.svelte'), 'utf8')
  const start = src.indexOf('function handleGlobalKeyDown(')
  const body = src.slice(start, src.indexOf('function getLastMessageId(', start))
  for (const def of dispatched) {
    for (const combo of def.combos) {
      const key = combo.split('+').pop()
      if (key.length !== 1 || def.legacy?.includes(combo)) continue
      assert.ok(!new RegExp(`case '${key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}':`).test(body), `case '${key}' for ${def.id} is hardcoded in App.svelte`)
    }
  }
})
