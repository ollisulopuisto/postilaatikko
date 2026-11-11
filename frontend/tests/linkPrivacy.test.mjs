import test from 'node:test'
import assert from 'node:assert/strict'
import { stripTrackingParams } from '../src/lib/utils/linkPrivacy.ts'

test('removes utm_* parameters', () => {
  const r = stripTrackingParams('https://example.com/page?utm_source=newsletter&utm_medium=email&id=42')
  assert.equal(r.url, 'https://example.com/page?id=42')
  assert.equal(r.stripped, true)
  assert.deepEqual(r.removed.sort(), ['utm_medium', 'utm_source'])
})

test('removes common click identifiers', () => {
  const r = stripTrackingParams('https://example.com/?fbclid=abc&gclid=def&msclkid=ghi&dclid=jkl')
  assert.equal(r.url, 'https://example.com/')
  assert.equal(r.stripped, true)
  assert.equal(r.removed.length, 4)
})

test('keeps tokens, signatures and non-tracking params', () => {
  const url = 'https://example.com/confirm?token=abc123&signature=xyz&ref=footer&source=email&lang=fi'
  const r = stripTrackingParams(url)
  assert.equal(r.url, url)
  assert.equal(r.stripped, false)
  assert.deepEqual(r.removed, [])
})

test('matching is case-insensitive on parameter names', () => {
  const r = stripTrackingParams('https://example.com/?UTM_SOURCE=x&gclid=y')
  assert.equal(r.url, 'https://example.com/')
  assert.equal(r.stripped, true)
})

test('preserves parameter order and fragment', () => {
  const r = stripTrackingParams('https://example.com/p?b=2&utm_campaign=sale&a=1#section')
  assert.equal(r.url, 'https://example.com/p?b=2&a=1#section')
})

test('leaves non-http(s) URLs untouched', () => {
  for (const url of ['mailto:a@b.fi?utm_source=x', 'ftp://h/f?utm_source=x', 'tel:+35840123']) {
    const r = stripTrackingParams(url)
    assert.equal(r.url, url)
    assert.equal(r.stripped, false)
  }
})

test('returns invalid input unchanged instead of throwing', () => {
  for (const bad of ['', 'not a url', 'https://', '::::']) {
    const r = stripTrackingParams(bad)
    assert.equal(r.url, bad)
    assert.equal(r.stripped, false)
  }
})

test('drops a dangling ? when every parameter is removed', () => {
  const r = stripTrackingParams('https://example.com/?utm_source=x#frag')
  assert.equal(r.url, 'https://example.com/#frag')
})

test('strips redirect-style trackers', () => {
  const r = stripTrackingParams('https://example.com/?mc_cid=1&mc_eid=2&igshid=3&spm=4&wickedid=5&vero_id=6')
  assert.equal(r.url, 'https://example.com/')
  assert.equal(r.removed.length, 6)
})
