import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'

// Run the actual registered callbacks, not a copy of their dispatch logic.
const source = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const script = source.match(/<script[^>]*>([\s\S]*?)<\/script>/)[1]
const ast = ts.createSourceFile('App.ts', script, ts.ScriptTarget.Latest, true)
const callbacks = new Map()
function visit(node) {
  if (ts.isCallExpression(node) && node.expression.getText(ast) === 'registerCommand') {
    const id = node.arguments[0]?.text
    if (id === 'mail.moveTo' || id === 'mail.copyTo') callbacks.set(id, node.arguments[1].getText(ast))
  }
  ts.forEachChild(node, visit)
}
visit(ast)

for (const [id, action] of [['mail.moveTo', 'move'], ['mail.copyTo', 'copy']]) {
  test(`${id} reads current focus at execution and opens the right picker`, () => {
    assert.ok(callbacks.has(id))
    const calls = []
    let pane = 'messageList'
    const context = {
      getFocusedPane: () => pane,
      selectedThreadId: 'thread-1',
      viewerRef: { toggleFolderPicker: mode => calls.push(['viewer', mode]) },
      messageListRef: {
        toggleMoveToDialog: () => calls.push(['list', 'move']),
        toggleCopyToDialog: () => calls.push(['list', 'copy']),
      },
    }
    const js = ts.transpileModule(`(${callbacks.get(id)})`, {
      compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
    }).outputText
    const run = vm.runInNewContext(js, context)
    run()
    pane = 'viewer'
    run()
    context.selectedThreadId = null
    run()
    assert.deepEqual(calls, [['list', action], ['viewer', action], ['list', action]])
  })
}
