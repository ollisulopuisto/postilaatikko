import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const composer = await readFile(new URL('../src/lib/components/composer/Composer.svelte', import.meta.url), 'utf8')
const insertion = composer.match(/ {2}function insertEmailTemplate\([\s\S]*?\n {2}}/)[0]
const functionJS = ts.transpileModule(insertion, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const historySource = await readFile(new URL('../src/lib/components/composer/draftHistory.ts', import.meta.url), 'utf8')
const historyJS = ts.transpileModule(historySource, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { DraftHistoryController } = await import(`data:text/javascript;base64,${Buffer.from(historyJS).toString('base64')}`)

for (const body of ['', 'Template body']) {
  test(`template insertion is undoable without losing the previous edit (${body ? 'subject and body' : 'subject only'})`, async () => {
    const nodes = []
    let current = ''
    let state = { subject: '', body: '', to: ['keep@example.test'], from: 'sender@example.test' }
    const backend = {
      commitBlobs: async (_key, parentId, label, payload) => {
        const nodeId = String(nodes.length + 1)
        const parent = nodes.find(n => n.nodeId === parentId)
        if (parent) parent.preferredChildId = nodeId
        nodes.push({ nodeId, parentId, preferredChildId: '', label, payload })
        current = nodeId
        return nodeId
      },
      tree: async () => ({ nodes: structuredClone(nodes), currentNodeId: current }),
      state: async id => nodes.find(n => n.nodeId === id).payload,
      checkout: async (_key, id) => { current = id },
    }
    const ctl = new DraftHistoryController('test', backend, () => JSON.stringify(state), payload => { state = JSON.parse(payload) }, err => { throw err })
    const flush = () => new Promise(resolve => setImmediate(resolve))
    ctl.commit('initial'); await flush()
    state.body = 'Earlier edit'
    ctl.commit('typing'); await flush()
    const before = structuredClone(state)
    // Run the actual Composer handler, exposing its lexical subject to the snapshot.
    const snapshotInsert = new Function('state', 'insertQuickReply', 'historyEdit', 'scheduleDraftSave', `return template => { ${functionJS.replace(/(?<![\w.])subject\b/g, 'state.subject')}; insertEmailTemplate(template) }`)(
      state, text => { state.body += text; ctl.commit('typing') }, label => ctl.commit(label), () => {},
    )
    snapshotInsert({ subject: 'Template subject', body })
    await flush()
    const after = structuredClone(state)
    assert.equal(after.subject, 'Template subject')
    assert.equal(after.body, 'Earlier edit' + body)
    assert.deepEqual(after.to, before.to)
    assert.equal(after.from, before.from)
    assert.equal(await ctl.undo(), true)
    assert.deepEqual(state, before)
    assert.equal(await ctl.redo(), true)
    assert.deepEqual(state, after)
  })
}
