import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const app = await read('../src/App.svelte')
const rail = await read('../src/lib/components/rail/ExtensionRail.svelte')
const controls = await read('../src/lib/components/common/WindowControls.svelte')
const composer = await read('../src/ComposerApp.svelte')

test('main window has no custom title bar row; the composer keeps its own', () => {
  assert.doesNotMatch(app, /<TitleBar|components\/common\/TitleBar/)
  assert.match(composer, /Custom Title Bar for frameless window/)
  assert.match(app, /<WindowControls|<ExtensionRail windowControls/)
})

test('traffic lights sit in the rail only in custom title bar mode, same condition as the old bar', () => {
  assert.match(app, /<ExtensionRail windowControls=\{getShowTitleBar\(\) && !getNativeTitleBar\(\) && effectiveTitleBarMode === 'custom'\} onCloseWindow=\{handleClose\}/)
  assert.match(rail, /\{#if windowControls\}[\s\S]*<WindowControls onClose=\{onCloseWindow\} \/>/)
  assert.match(rail, /windowControls \? 'w-\[3\.75rem\]' : 'w-12 pt-2'/)
})

test('traffic lights are in the standard Mac order and opt out of dragging', () => {
  const order = ['aria.closeWindow', 'aria.minimizeWindow', 'aria.maximizeWindow'].map(key => controls.indexOf(key))
  assert.ok(order.every(i => i > -1))
  assert.deepEqual([...order].sort((a, b) => a - b), order)
  assert.match(controls, /--wails-draggable: no-drag/)
  assert.match(rail, /h-10[^\n]*--wails-draggable: drag|--wails-draggable: drag[^\n]*\n\s*<WindowControls/)
})

test('history arrows and the new window button live in the viewer pane tab row', () => {
  const row = app.slice(app.indexOf('<MessageHistoryBar'), app.indexOf('{#if activeMailWorkspace === \'primary\'}\n      <ConversationViewer'))
  assert.match(row, /mailWindow\.newWindow/)
  assert.equal((app.match(/<MessageHistoryBar/g) || []).length, 1)
})
