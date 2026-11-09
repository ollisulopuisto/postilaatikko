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

test('Cmd/Ctrl+K opens the palette, with Cmd/Ctrl+P kept', () => {
  assert.match(shortcuts, /COMMAND_PALETTE[\s\S]{0,200}e\.key === 'k'/)
  assert.match(shortcuts, /COMMAND_PALETTE[\s\S]{0,200}e\.key === 'p'/)
  assert.match(registry, /id: 'app\.commandPalette'[^\n]*'Mod\+k'/)
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

test('recent commands are stored newest first, capped at eight', () => {
  assert.match(store, /RECENT_MAX = 8/)
  assert.match(store, /\[id, \.\.\.getRecentCommands\(\)\.filter\(entry => entry !== id\)\]\.slice\(0, RECENT_MAX\)/)
})
