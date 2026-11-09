// Command palette: Cmd/Ctrl+K opens it, repeatable commands keep it open, and
// the lesser-used mail commands are listed. Source-level checks, no DOM.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const read = file => readFileSync(new URL(`../src/lib/${file}`, import.meta.url), 'utf8')
const registry = read('keyboard/registry.ts')
const shortcuts = read('keyboard/shortcuts.ts')
const store = read('stores/commandPalette.svelte.ts')

function commandLine(id) {
  const line = registry.split('\n').find(l => l.includes(`{ id: '${id}',`))
  assert.ok(line, `${id} is in CORE_COMMANDS`)
  return line
}

test('Cmd/Ctrl+K stays Archive; the palette keeps Cmd/Ctrl+P', () => {
  assert.doesNotMatch(shortcuts, /COMMAND_PALETTE[^\n]*\n[^\n]*'k'/)
  assert.match(shortcuts, /COMMAND_PALETTE[\s\S]{0,120}e\.key === 'p'/)
  assert.match(registry, /id: 'app\.commandPalette'[^\n]*combos: \['Mod\+p', 'Mod\+Shift\+p'\]/)
  assert.match(registry, /id: 'mail\.archive'[^\n]*combos: \['Mod\+k'\]/)
})

test('repeatable commands keep the palette open', () => {
  for (const id of ['app.undo', 'theme.toggle', 'mail.star', 'mail.markRead', 'mail.markUnread']) {
    assert.match(commandLine(id), /keepOpen: true/, id)
  }
  assert.doesNotMatch(commandLine('mail.archive'), /keepOpen/)
  assert.match(store, /if \(!getCommand\(id\)\?\.keepOpen\) closeCommandPalette\(\)/)
})

test('lesser-used mail commands are in the palette; permanent delete stays out', () => {
  for (const id of ['mail.toggleRead', 'mail.readLater', 'mail.replyLater', 'mail.moveTo', 'mail.copyTo']) {
    assert.doesNotMatch(commandLine(id), /palette: false/, id)
    assert.match(commandLine(id), /category: 'commandPalette\.categories\.mail'/, id)
  }
  assert.match(commandLine('mail.deletePermanently'), /palette: false/)
})

test('recents are reactive, newest first, capped at eight, and only the palette records them', () => {
  const palette = readFileSync(new URL('../src/lib/components/command/CommandPalette.svelte', import.meta.url), 'utf8')
  assert.match(store, /RECENT_MAX = 8/)
  assert.match(store, /let recent = \$state<string\[\]>/)
  assert.match(store, /recent = \[id, \.\.\.recent\.filter\(entry => entry !== id\)\]\.slice\(0, RECENT_MAX\)/)
  // executeCommand (keyboard shortcuts, hidden commands) must not write history.
  const execute = store.slice(store.indexOf('export async function executeCommand'))
  assert.doesNotMatch(execute, /recordRecentCommand/)
  assert.match(palette, /recordRecentCommand\(cmd\.id\)/)
})

test('after a keep-open command the selection follows it by id', () => {
  const palette = readFileSync(new URL('../src/lib/components/command/CommandPalette.svelte', import.meta.url), 'utf8')
  assert.match(palette, /selectedIndex = Math\.max\(0, filteredCommands\.findIndex\(item => item\.id === cmd\.id\)\)/)
})
