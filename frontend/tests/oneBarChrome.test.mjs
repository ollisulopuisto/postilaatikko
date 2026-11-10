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

test('double-clicking the drag strip still toggles maximise, and the buttons do not drag', () => {
  assert.match(rail, /--wails-draggable: drag"[^>]*ondblclick=\{\(\) => void toggleMaximise\(\)\}/)
  assert.match(rail, /import \{ toggleMaximise \}/)
})

test('the zoom button and the strip share one maximised state, so the label never goes stale', async () => {
  const state = await read('../src/lib/windowState.svelte.ts')
  assert.match(state, /WindowIsMaximised\(\)/)
  assert.match(controls, /windowState\.maximised \? \$_\('window\.restore'\)/)
  assert.match(controls, /onclick=\{\(\) => void toggleMaximise\(\)\}/)
  assert.doesNotMatch(controls, /\$state\(false\)[^\n]*\n[^\n]*isMaximized|let isMaximized/)
})

test('settleMaximised keeps reading until a delayed native toggle shows the new state', async () => {
  const { default: ts } = await import('typescript')
  const source = await read('../src/lib/windowState.svelte.ts')
  const start = source.indexOf('export async function settleMaximised')
  const end = source.indexOf('export async function toggleMaximise')
  const js = ts.transpileModule(source.slice(start, end), { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
  const { settleMaximised } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)
  const noWait = async () => {}
  // The native window reports the old state twice before the zoom finishes.
  const reads = [false, false, true]
  let calls = 0
  assert.equal(await settleMaximised(async () => reads[Math.min(calls++, 2)], false, noWait), true)
  assert.equal(calls, 3)
  // A toggle that never changes the state gives up with the last real value.
  assert.equal(await settleMaximised(async () => false, false, noWait, 3), false)
})
