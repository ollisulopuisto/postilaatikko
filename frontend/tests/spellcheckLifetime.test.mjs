import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function client() {
  let source = await readFile(new URL('../src/lib/spellcheck/client.ts', import.meta.url), 'utf8')
  source = source.replace(/^import .*$/gm, '').replace("new URL('./worker.ts', import.meta.url)", "'worker.ts'")
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
  const workers = []
  class Worker {
    messages = []
    terminated = false
    constructor() { workers.push(this) }
    postMessage(message) { this.messages.push(message) }
    terminate() { this.terminated = true }
  }
  const factory = new Function('Worker', 'appLocaleToDict', 'logger', outputText.replace('export const spellcheck', 'const spellcheck') + '\nreturn spellcheck')
  return { api: factory(Worker, x => x === 'en' ? 'en' : null, { error() {}, info() {} }), workers }
}

async function settled(promise) {
  const pending = Symbol('pending')
  const result = await Promise.race([promise, new Promise(resolve => setImmediate(() => resolve(pending)))])
  assert.notEqual(result, pending, 'pending spellcheck request must settle')
  return result
}

test('destroy settles pending checks and suggestions without retaining resolvers', async () => {
  const { api, workers } = await client()
  api.setLanguages(['en'])
  const check = api.check(['hello'])
  const suggest = api.suggest('teh')
  api.destroy()
  assert.deepEqual(await settled(check), new Set())
  assert.deepEqual(await settled(suggest), [])
  assert.equal(workers[0].terminated, true)
  assert.equal(api.isActive, false)
  api.destroy()
})

for (const failure of ['onerror', 'onmessageerror']) {
  test(`${failure} settles pending requests and permits a fresh worker`, async () => {
    const { api, workers } = await client()
    api.setLanguages(['en'])
    const check = api.check(['hello'])
    workers[0][failure]({ message: 'worker failed' })
    assert.deepEqual(await settled(check), new Set())
    assert.equal(workers[0].terminated, true)
    assert.equal(api.isActive, false)
    api.setLanguages(['en'])
    assert.equal(workers.length, 2)
    const next = api.suggest('teh')
    const request = workers[1].messages.at(-1)
    workers[1].onmessage({ data: { type: 'suggested', id: request.id, suggestions: ['the'] } })
    assert.deepEqual(await settled(next), ['the'])
    api.destroy()
  })
}

test('normal replies still resolve only their request', async () => {
  const { api, workers } = await client()
  api.setLanguages(['en'])
  const check = api.check(['teh'])
  const suggest = api.suggest('teh')
  const messages = workers[0].messages
  workers[0].onmessage({ data: { type: 'checked', id: messages[1].id, misspelled: ['teh'] } })
  workers[0].onmessage({ data: { type: 'suggested', id: messages[2].id, suggestions: ['the'] } })
  assert.deepEqual(await settled(check), new Set(['teh']))
  assert.deepEqual(await settled(suggest), ['the'])
  api.destroy()
})

test('disabling all spellcheck languages settles work before going idle', async () => {
  const { api } = await client()
  api.setLanguages(['en'])
  const check = api.check(['hello'])
  api.setLanguages(['fi'])
  assert.deepEqual(await settled(check), new Set())
  assert.equal(api.isActive, false)
})

test('late events from a destroyed worker cannot mark a replacement ready or kill it', async () => {
  const { api, workers } = await client()
  api.setLanguages(['en'])
  const old = workers[0]
  api.destroy()
  api.setLanguages(['en'])
  let readyCount = 0
  const unsubscribe = api.onReady(() => readyCount++)
  old.onmessage({ data: { type: 'ready', loaded: ['en'], selfTest: true } })
  old.onerror({ message: 'late failure' })
  old.onmessageerror({})
  assert.equal(readyCount, 0)
  assert.equal(api.isActive, true)
  workers[1].onmessage({ data: { type: 'ready', loaded: ['en'], selfTest: true } })
  assert.equal(readyCount, 1)
  unsubscribe()
  api.destroy()
})
