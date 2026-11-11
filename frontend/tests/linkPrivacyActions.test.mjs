import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import {stripTrackingParams} from '../src/lib/utils/linkPrivacy.ts'

const current = fs.readFileSync(new URL('../src/lib/components/viewer/EmailBody.svelte', import.meta.url), 'utf8')
const source = process.env.LINK_ACTION_SOURCE ? fs.readFileSync(process.env.LINK_ACTION_SOURCE, 'utf8') : current
function action(name) {
  const match = source.match(new RegExp(`(?:async )?function ${name}\\((.*?)\\) \\{([\\s\\S]*?)\\n {2}\\}`))
  return {args: match[1].replace(/: string/g, ''), body: match[2]}
}
const tracked = 'https://example.com/?token=kept&utm_source=email'
const clean = 'https://example.com/?token=kept'

test('DNS action cleans by default and preserves original only on explicit bypass', async () => {
  const {args,body} = action('openViaCloudflareDNS')
  const run = new Function('ResolveLinkViaCloudflareDNS', 'cleanLinkUrl', `return async function(${args}) {let ctxMenuUrl=${JSON.stringify(tracked)},chainOriginal='',ctxMenuVisible=true,chainDialogVisible=false,chainLoading=false,chainHops=[],chainFinal='',chainError=''; ${body}; return {chainLoading,chainDialogVisible}}`)
  const calls=[]
  const invoke=run(async url => {calls.push(url);return {hops:[],finalURL:url}}, url=>stripTrackingParams(url).url)
  assert.deepEqual(await invoke(), {chainLoading:false,chainDialogVisible:true})
  await invoke(true)
  assert.deepEqual(calls,[clean,tracked])
})

test('final open cleans, explicit original open preserves bytes, and both close dialog', () => {
  const {args,body}=action('openChainURL')
  const calls=[]
  const invoke=new Function('safeOpenURL','cleanLinkUrl',`return function(${args}) {let chainDialogVisible=true; ${body}; return chainDialogVisible}`)(url=>calls.push(url),url=>stripTrackingParams(url).url)
  assert.equal(invoke(tracked),false)
  assert.equal(invoke(tracked,true),false)
  assert.deepEqual(calls,[clean,tracked])
  assert.match(current,/openChainURL\(chainOriginal, true\)/)
})

test('final copy cleans tracking added by a redirect and closes dialog', async () => {
  const {args,body}=action('copyChainURL')
  const calls=[]
  const invoke=new Function('navigator','cleanLinkUrl',`return async function(${args}) {let chainDialogVisible=true; ${body}; return chainDialogVisible}`)({clipboard:{writeText:async url=>calls.push(url)}},url=>stripTrackingParams(url).url)
  assert.equal(await invoke(tracked),false)
  assert.deepEqual(calls,[clean])
})
