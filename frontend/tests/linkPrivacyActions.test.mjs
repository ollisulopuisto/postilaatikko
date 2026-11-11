import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import {stripTrackingParams} from '../src/lib/utils/linkPrivacy.ts'
import {linkForAction, openResolvedLink, copyResolvedLink} from '../src/lib/utils/linkPrivacyActions.ts'

const source = fs.readFileSync(new URL('../src/lib/components/viewer/EmailBody.svelte', import.meta.url), 'utf8')
const tracked = 'https://example.com/?token=kept&utm_source=email'
const cleaned = 'https://example.com/?token=kept'
const clean = url => stripTrackingParams(url).url

test('DNS request cleans by default and preserves bytes only on explicit original bypass', () => {
  assert.equal(linkForAction(tracked, false, clean), cleaned)
  assert.equal(linkForAction(tracked, true, clean), tracked)
  assert.match(source, /ResolveLinkViaCloudflareDNS\(linkForAction\(chainOriginal, original, cleanLinkUrl\)\)/)
  assert.match(source, /openViaCloudflareDNS\(true\)/)
})

test('final open cleans redirect trackers, while explicit original open bypasses cleaning', () => {
  const calls=[]
  openResolvedLink(tracked, false, clean, url=>calls.push(url))
  openResolvedLink(tracked, true, clean, url=>calls.push(url))
  assert.deepEqual(calls,[cleaned,tracked])
  assert.match(source,/openResolvedLink\(url, original, cleanLinkUrl, safeOpenURL\)/)
  assert.match(source,/openChainURL\(chainOriginal, true\)/)
})

test('final copy cleans parameters added by a redirect', async () => {
  const calls=[]
  await copyResolvedLink(tracked,clean,async url=>calls.push(url))
  assert.deepEqual(calls,[cleaned])
  assert.match(source,/copyResolvedLink\(url, cleanLinkUrl,/)
})
