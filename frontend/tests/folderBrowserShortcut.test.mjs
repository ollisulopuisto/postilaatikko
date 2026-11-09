import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
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
const pane = await server.ssrLoadModule('/src/lib/sidebar/folderPane.ts')

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
  assert.match(composer, /\(e\.key === 'D' \|\| e\.key === 'd'\) && e\.shiftKey && \(e\.ctrlKey \|\| e\.metaKey\)/)
  assert.doesNotMatch(composer, /e\.key === 'd' && \(e\.ctrlKey \|\| e\.metaKey\)/)
  assert.match(app, /case 'd':[\s\S]*?if \(e\.shiftKey\) return[\s\S]*?executeCommand\('app\.folders'\)/)
  assert.match(app, /registerCommand\('app\.folders', \(\) => toggleFolderBrowser\(\)\)/)
})

test('wide windows share one column: the open pane takes the width and the other folds to a strip', () => {
  assert.match(app, /const sharedColumn = \$derived\(getLayoutMode\(\) === 'full'\)/)
  assert.match(app, /sharedPaneWidths\(listWidth, contextPaneMode\)/)
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


const state = (layoutMode, over = {}) => ({ contextPaneMode: 'messages', layoutMode, responsiveView: 'default', ...over })

test('narrow window: every way out of the tree restores the list and hides the overlay', () => {
  const opened = pane.openFolderTree(state('narrow'))
  assert.equal(opened.contextPaneMode, 'folders')
  assert.equal(opened.responsiveView, 'sidebar')
  // Back button, scrim, close button, Escape and Cmd-D all call the same close.
  const closed = pane.closeFolderTree(opened)
  assert.equal(closed.contextPaneMode, 'messages')
  assert.equal(closed.responsiveView, 'default')
  assert.deepEqual(pane.toggleFolderTree(opened), closed)
  // Wide window: opening never touches the overlay state, closing restores the list.
  const wide = pane.openFolderTree(state('full'))
  assert.equal(wide.responsiveView, 'default')
  assert.equal(pane.closeFolderTree(wide).contextPaneMode, 'messages')
  // Toggling twice returns to the start.
  assert.deepEqual(pane.toggleFolderTree(pane.toggleFolderTree(state('narrow'))), state('narrow'))
})

test('App routes Back, scrim and the tree close button through one close handler', () => {
  assert.match(app, /onBack=\{closeFolderBrowser\}/)
  assert.match(app, /onclick=\{closeFolderBrowser\}\s*aria-label=\{\$_\('aria\.closeSidebar'\)\}/)
  assert.match(app, /onCloseTree=\{closeFolderBrowser\}/)
  assert.doesNotMatch(app, /onBack=\{hideSidebar\}/)
})

test('Escape: the tree consumes it only when nothing else owns it, and clears a search first', () => {
  const base = { treeOpen: true, query: '', defaultPrevented: false, overlayOpen: false }
  assert.equal(pane.treeEscapeAction(base), 'close')
  assert.equal(pane.treeEscapeAction({ ...base, query: 'inbox' }), 'clear-query')
  assert.equal(pane.treeEscapeAction({ ...base, treeOpen: false }), 'ignore')
  assert.equal(pane.treeEscapeAction({ ...base, overlayOpen: true }), 'ignore')
  assert.equal(pane.treeEscapeAction({ ...base, defaultPrevented: true }), 'ignore')
  // Capture-phase listener that stops propagation, so the window handler that
  // clears the selected conversation never sees a tree-dismissing Escape.
  assert.match(sidebar, /<svelte:window onkeydowncapture=/)
  assert.match(sidebar, /e\.preventDefault\(\)\s*e\.stopPropagation\(\)/)
})

test('divider drag measures from the column edge, so folders mode does not collapse the width', () => {
  const columnLeft = 64 // right edge of the rail, the same in both modes
  const min = 280, max = 700
  // Drag to x=484 -> 420px wide, whichever pane is open.
  assert.equal(pane.dragListWidth(484, columnLeft, min, max), 420)
  // The old origin in folders mode was the folded list strip, far to the right of the column edge.
  const widths = pane.sharedPaneWidths(420, 'folders')
  const shiftedLeft = columnLeft + widths.folders
  assert.equal(pane.dragListWidth(484, shiftedLeft, min, max), min, 'documents the bug the shared origin avoids')
  assert.equal(pane.dragListWidth(5000, columnLeft, min, max), max)
  // Pane widths always add up to the column width.
  for (const mode of ['folders', 'messages']) {
    const w = pane.sharedPaneWidths(420, mode)
    assert.equal(w.folders + w.messages, 420)
  }
  assert.match(app, /\(sharedColumn \? contextColumnRef : messageListContainerRef\)\?\.getBoundingClientRect\(\)\.left/)
})

test('review fixes: close restores list focus, tabs and rail honor Cmd-D, strips expose state', () => {
  assert.match(app, /tick\(\)\.then\(\(\) => messageListContainerRef\?\.focus\(\)\)/)
  assert.match(app, /if \(e\.key\.toLowerCase\(\) === 'd' && !e\.shiftKey\) \{\s*e\.preventDefault\(\)\s*toggleFolderBrowser\(\)/)
  assert.match(app, /foldersOpen=\{contextPaneMode === 'folders'/)
  assert.match(rail, /active=\{foldersOpen\}/)
  assert.match(app, /aria-expanded="false"/)
})

test('Escape with the tree open: Undo Send countdown and the composer keep precedence', () => {
  const base = { treeOpen: true, query: '', defaultPrevented: false, overlayOpen: false }
  // Pending send or open composer -> tree ignores Escape, so App's pendingCancel / Composer handleClose run.
  assert.equal(pane.treeEscapeAction({ ...base, ownedElsewhere: true }), 'ignore')
  assert.equal(pane.treeEscapeAction({ ...base, query: 'x', ownedElsewhere: true }), 'ignore')
  assert.equal(pane.treeEscapeAction({ ...base, ownedElsewhere: false }), 'close')
  // Wiring: App passes both conditions, Sidebar feeds it to the decision.
  assert.match(app, /escapeOwnedElsewhere=\{pendingSends\.length > 0 \|\| showComposer\}/)
  assert.match(sidebar, /ownedElsewhere: escapeOwnedElsewhere/)
  // And App's handler still cancels the pending send on Escape.
})

test('one Escape, one owner: tree + pending send + composer', async () => {
  const composer = await readFile(new URL('../src/lib/components/composer/Composer.svelte', import.meta.url), 'utf8')
  const base = { treeOpen: true, query: '', defaultPrevented: false, overlayOpen: false, ownedElsewhere: true }
  // 1. Tree stands down.
  assert.equal(pane.treeEscapeAction(base), 'ignore')
  // 2. App's pending-send cancel consumes the event for every other window listener.
  assert.match(app, /e\.key === 'Escape' && !ps\.dispatching\) \{[^}]*e\.preventDefault\(\)\s*e\.stopImmediatePropagation\(\)\s*void pendingCancel\(ps\)/)
  // 3. If listener order ever flips, the Composer still yields to a handled Escape.
  assert.match(composer, /if \(e\.key === 'Escape'\) \{[^}]*if \(e\.defaultPrevented\) return\s*handleClose\(\)/)
})
