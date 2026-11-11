// Link privacy: strip known tracking parameters from URLs at use time.
// The stored message and the displayed URL are never mutated — only the URL
// that gets opened in the browser or copied to the clipboard is cleaned.

// Non-utm tracking parameter names (lowercase). Any utm_* name is stripped by
// prefix match, so unlisted variants like utm_reader_id are covered too.
const TRACKING_PARAMS = new Set([
  'fbclid',
  'gclid',
  'dclid',
  'msclkid',
  'twclid',
  'li_fat_id',
  'mc_cid',
  'mc_eid',
  'igshid',
  'spm',
  'scm',
  '_hsenc',
  '_hsmi',
  'hsctatracking',
  'mkt_tok',
  'vero_id',
  'wickedid',
  'nr_email_referer',
])

export interface StripResult {
  url: string
  stripped: boolean
  removed: string[]
}

function isTrackingParam(name: string): boolean {
  const lower = name.toLowerCase()
  return lower.startsWith('utm_') || TRACKING_PARAMS.has(lower)
}

// stripTrackingParams removes known tracking parameters from an http(s) URL.
// It filters raw query segments instead of rebuilding the query with
// URLSearchParams, so kept parameters survive byte-for-byte — re-encoding
// percent escapes (%2f → %2F, %20 → +) would break signatures computed over
// the original query string. Non-http(s) input and URLs without tracking
// parameters are returned unchanged with stripped: false. Never throws.
export function stripTrackingParams(input: string): StripResult {
  const unchanged: StripResult = { url: input, stripped: false, removed: [] }
  if (!input || !/^https?:\/\//i.test(input)) return unchanged
  const hashIdx = input.indexOf('#')
  const fragment = hashIdx >= 0 ? input.slice(hashIdx) : ''
  const withoutFragment = hashIdx >= 0 ? input.slice(0, hashIdx) : input
  const queryIdx = withoutFragment.indexOf('?')
  if (queryIdx < 0) return unchanged
  const base = withoutFragment.slice(0, queryIdx)
  const query = withoutFragment.slice(queryIdx + 1)
  if (!query) return unchanged
  const removed: string[] = []
  const kept: string[] = []
  for (const segment of query.split('&')) {
    const eqIdx = segment.indexOf('=')
    const rawName = eqIdx >= 0 ? segment.slice(0, eqIdx) : segment
    let name = rawName
    try {
      name = decodeURIComponent(rawName)
    } catch {
      // Keep the raw name when it is not valid percent-encoding.
    }
    if (isTrackingParam(name)) {
      removed.push(name)
      continue
    }
    kept.push(segment)
  }
  if (removed.length === 0) return unchanged
  const url = kept.length > 0 ? `${base}?${kept.join('&')}${fragment}` : `${base}${fragment}`
  return { url, stripped: true, removed }
}
