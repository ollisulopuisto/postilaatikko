import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
const source = await readFile(new URL('../src/lib/components/viewer/reactionLayout.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { reactionLayout } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const target = { id:'target', accountId:'account', messageId:'<target@example.com>' }
const reaction = (id, extra={}) => ({id, accountId:'account', messageId:`<${id}@example.com>`, fromName:'Peer', fromEmail:'peer@example.com', reaction:{emoji:'👍', targetMessageId:'target@example.com', provider:'gmail'}, ...extra})
test('reactions attach to the exact original, not an intervening previous message', () => {
  const rows = [target, {id:'intervening', accountId:'account', messageId:'<other@example.com>'}, reaction('r')]
  const result = reactionLayout(rows)
  assert.deepEqual(result.messages.map(row=>row.id), ['target','intervening'])
  assert.equal(result.groups.get('target')[0].emoji,'👍')
  assert.equal(result.groups.get('target')[0].messages[0].id,'r')
  assert.equal(rows.length,3, 'original model was modified')
})
test('unknown targets, other accounts, invalid metadata, attachments and encrypted messages stay visible',()=>{
  for (const item of [reaction('r',{accountId:'other'}),reaction('r',{hasAttachments:true}),reaction('r',{hasSMIME:true}),reaction('r',{reaction:{emoji:'👍',targetMessageId:'missing@example.com',provider:'gmail'}}),reaction('r',{reaction:{emoji:'👍',targetMessageId:'target@example.com',provider:'gmail',invalid:true}})]) {
    assert.equal(reactionLayout([target,item]).messages.length,2)
  }
})
test('same-emoji reactions group, copies deduplicate, and ordinary emoji replies stay messages',()=>{
  const a=reaction('a');const b=reaction('b',{fromName:'Another',fromEmail:'another@example.com'})
  const result=reactionLayout([target,a,{...a,id:'copy'},b,{id:'ordinary',accountId:'account',bodyText:'👍'}])
  assert.equal(result.groups.get('target')[0].messages.length,2)
  assert.deepEqual(result.messages.map(row=>row.id),['target','ordinary'])
  assert.ok(result.hidden.has('copy'))
})
