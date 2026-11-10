// Bare single-key registry shortcuts (Gmail-style 'd', 'e', 's', ...) act on
// the mail list. When an extension (Calendar, Contacts) owns the rail, the
// mail list is hidden and those keys must NOT fire — pressing 'd' in Contacts
// must not trash an invisible message. The bare-key tail of
// handleGlobalKeyDown therefore requires isMailActive(), the same guard the
// modified-key registry path already has. Global single keys ('?' shortcuts
// help) are handled earlier and stay available on every pane.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
const registry = await readFile(new URL('../src/lib/keyboard/registry.ts', import.meta.url), 'utf8')

test('bare-key registry tail requires the mail rail to be active', () => {
  const tail = app.match(/Single-key shortcuts[\s\S]{0,400}?runRegistryShortcut\(e\)/)
  assert.ok(tail, 'bare-key registry tail exists')
  assert.match(tail[0], /if \(!isMailActive\(\)\) return/, 'bare-key tail is guarded by isMailActive()')
})

test("bare 'd' stays bound to mail.trash — the guard, not the binding, protects other panes", () => {
  assert.match(registry, /id: 'mail\.trash'[^\n]*'d'/, "bare 'd' is still the trash shortcut")
  assert.match(registry, /id: 'mail\.trash'[^\n]*panes: MAIL/, 'mail.trash is mail-scoped')
})

test("global '?' shortcuts help is handled before the guard and stays on every pane", () => {
  assert.match(app, /KEY\.SHORTCUTS_HELP\(e\)/, "'?' help has its own early handler")
  assert.match(registry, /id: 'app\.shortcuts'[^\n]*combos: \['\?'\]/, "'?' remains the help combo")
})
