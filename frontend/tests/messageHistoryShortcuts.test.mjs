import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'
const source = await readFile(new URL('../src/lib/keyboard/shortcuts.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { HISTORY_BACK, HISTORY_FORWARD } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const key = (value, extra = {}) => ({ key: value, metaKey: true, ctrlKey: false, altKey: false, shiftKey: false, ...extra })
test('only plain Cmd-Left/Right navigate history, never Ctrl, brackets or modified variants', () => {
  assert.equal(HISTORY_BACK(key('ArrowLeft')), true)
  assert.equal(HISTORY_FORWARD(key('ArrowRight')), true)
  for (const predicate of [HISTORY_BACK, HISTORY_FORWARD]) {
    for (const value of ['[', ']', 'Home', 'End', 'z', 'ArrowUp', 'ArrowDown']) assert.equal(predicate(key(value)), false)
    for (const value of ['ArrowLeft', 'ArrowRight']) {
      for (const extra of [{metaKey:false}, {ctrlKey:true}, {shiftKey:true}, {altKey:true}, {metaKey:false,ctrlKey:true}]) assert.equal(predicate(key(value, extra)), false)
    }
  }
})
test('history handling precedes tab guard and rejects editing, overlays, composition and handled events', async () => {
  const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
  const handler = app.slice(app.indexOf('function handleGlobalKeyDown('), app.indexOf('// Track Left Alt press'))
  const nav = handler.slice(0, handler.indexOf('// Secondary workspaces'))
  for (const guard of ['e.defaultPrevented', 'e.isComposing', "getActiveExtension() !== 'mail'", 'showComposer', 'isInputElement(e.target)', 'isInputElement(e.composedPath()[0])', 'isInputElement(document.activeElement)', 'isDialogGuardActive()']) assert.ok(nav.includes(guard), guard)
  assert.match(nav, /navigateMessageHistory\(KEY.HISTORY_BACK\(e\) \? -1 : 1\)/)
  assert.doesNotMatch(nav, /UndoOperation|ClearDraftHistory|pendingCancel|runListAction/)
})
