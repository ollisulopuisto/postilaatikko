import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const read = (p) => readFile(new URL(p, import.meta.url), 'utf8')
const registry = await read('../src/lib/keyboard/registry.ts')
const app = await read('../src/App.svelte')
const composer = await read('../src/lib/components/composer/Composer.svelte')
const sidebar = await read('../src/lib/components/sidebar/Sidebar.svelte')
const rail = await read('../src/lib/components/rail/ExtensionRail.svelte')

test('Mod+D toggles the folder tree and composer detach moved to Mod+Shift+D', () => {
  assert.match(registry, /id: 'app\.folders'[^\n]*combos: \['Mod\+d'\]/)
  assert.match(registry, /id: 'composer\.detach'[^\n]*combos: \['Mod\+Shift\+d'\]/)
  assert.match(registry, /\{ id: 'app\.folders', category: [^\n]*shortcutId: 'app\.folders' \}/)
  assert.match(composer, /e\.key === 'D' && e\.shiftKey && \(e\.ctrlKey \|\| e\.metaKey\)/)
  assert.doesNotMatch(composer, /e\.key === 'd' && \(e\.ctrlKey \|\| e\.metaKey\)/)
  assert.match(app, /case 'd':[\s\S]*?if \(e\.shiftKey\) return[\s\S]*?executeCommand\('app\.folders'\)/)
  assert.match(app, /registerCommand\('app\.folders', \(\) => toggleFolderBrowser\(\)\)/)
})

test('wide windows share one column: the open pane takes the width and the other folds to a strip', () => {
  assert.match(app, /const sharedColumn = \$derived\(getLayoutMode\(\) === 'full'\)/)
  assert.match(app, /listWidth - FOLDED_PANE_WIDTH/)
  assert.match(app, /class:pane-folded=\{sharedColumn && contextPaneMode !== 'folders'\}/)
  assert.match(app, /class:pane-folded=\{sharedColumn && contextPaneMode === 'folders'\}/)
  assert.match(app, /treeMode=\{contextPaneMode === 'folders'\}/)
})

test('the tree fills the pane, Escape and close return to the list, and the rail has a Folders button', () => {
  assert.match(sidebar, /\{#if showAllFolders \|\| treeMode\}/)
  assert.match(sidebar, /if \(treeMode\) onCloseTree\?\.\(\)/)
  assert.match(rail, /icon="mdi:folder-multiple-outline"[^\n]*executeCommand\('app\.folders'\)/)
  // Picking a folder already switches the pane back to the message list.
  assert.match(app, /function handleFolderSelect\([\s\S]*?contextPaneMode = 'messages'/)
})
