import { test, after } from 'node:test'
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { readFile } from 'node:fs/promises'
import { createServer } from 'vite'
const root = fileURLToPath(new URL('..', import.meta.url))
const server = await createServer({ root, configFile: path.join(root, 'vite.config.ts'), logLevel: 'error', server: { middlewareMode: true, hmr: false, watch: null }, appType: 'custom', optimizeDeps: { noDiscovery: true, include: [] } })
after(() => server.close())
const { COMMAND_PALETTE } = await server.ssrLoadModule('/src/lib/keyboard/shortcuts.ts')
const key = (key, extra = {}) => ({ key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...extra })
test('Mac palette uses Command, physical Control remains text editing', () => {
  for (const letter of ['k', 'K', 'p', 'P']) {
    assert.equal(COMMAND_PALETTE(key(letter, { ctrlKey: true }), true), false)
    assert.equal(COMMAND_PALETTE(key(letter, { metaKey: true }), true), true)
    assert.equal(COMMAND_PALETTE(key(letter, { metaKey: true, ctrlKey: true }), true), false)
    assert.equal(COMMAND_PALETTE(key(letter, { ctrlKey: true }), false), true)
  }
})
test('focused text retains registry Undo before mail dispatch', async () => {
  const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
  const body = app.slice(app.indexOf('function runRegistryShortcut('), app.indexOf('function handleGlobalKeyDown('))
  assert.match(body, /def\.id === 'app.undo'/)
  for (const target of ['e.target', 'e.composedPath()[0]', 'document.activeElement']) assert.ok(body.includes(`isInputElement(${target})`), target)
  assert.ok(body.indexOf("def.id === 'app.undo'") < body.indexOf('e.preventDefault()'))
})

test('actual App registry dispatcher never calls mail undo for focused text', async () => {
  const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
  const body = app.slice(app.indexOf('function runRegistryShortcut('), app.indexOf('function handleGlobalKeyDown('))
  const ts = (await import('typescript')).default
  const js = ts.transpileModule(body, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
  const { resolveShortcut } = await server.ssrLoadModule('/src/lib/keyboard/dispatch.ts')
  let calls = 0
  const field = { editable: true }
  const doc = { activeElement: null }
  const run = new Function('resolveShortcut', 'getFocusedPane', 'KEY_HANDLERS', 'isInputElement', 'document', 'selectedThreadId', js + ';return runRegistryShortcut')(resolveShortcut, () => 'messageList', { 'app.undo': () => calls++ }, value => !!value?.editable, doc, 'thread')
  for (const route of ['target', 'path', 'active']) {
    doc.activeElement = route === 'active' ? field : null
    const e = { ...key('z', { metaKey: true }), target: route === 'target' ? field : {}, composedPath: () => [route === 'path' ? field : {}], preventDefault: () => { throw Error('text undo consumed') } }
    assert.equal(run(e), false)
  }
  assert.equal(calls, 0)
  doc.activeElement = null
  let prevented = false
  assert.equal(run({ ...key('z', { metaKey: true }), target: {}, composedPath: () => [{}], preventDefault: () => { prevented = true } }), true)
  assert.equal(calls, 1)
  assert.equal(prevented, true)
})

test('native Edit Undo stays with focused text, including an empty history', async () => {
  const app = await readFile(new URL('../src/App.svelte', import.meta.url), 'utf8')
  const body = app.split("EventsOn('undo:requested', () => {")[1].split('\n    })')[0]
  const ts = (await import('typescript')).default
  const js = ts.transpileModule(body, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
  let mail = 0, text = 0, composer = 0
  class Element { constructor(editable, inComposer = false) { this.editable = editable; this.inComposer = inComposer } closest() { return this.inComposer } }
  const doc = { activeElement: null, focused: true, hasFocus() { return this.focused }, execCommand(command) { assert.equal(command, 'undo'); text++; return false } }
  const handler = new Function('document', 'HTMLElement', 'showComposer', 'window', 'isInputElement', 'handleGlobalUndo', js)
  const run = (showComposer = false) => handler(doc, Element, showComposer, { dispatchEvent() { composer++ } }, value => !!value?.editable, () => { mail++ })
  doc.activeElement = new Element(true)
  run(); assert.equal(text, 1); assert.equal(mail, 0)
  doc.activeElement = new Element(true, true)
  run(true); assert.equal(composer, 1); assert.equal(mail, 0)
  doc.activeElement = new Element(false)
  run(); assert.equal(mail, 1)
  doc.focused = false; run(); assert.equal(mail, 1)
})
