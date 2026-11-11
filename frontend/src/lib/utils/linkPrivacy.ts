// Strip tracking parameters from links (link privacy).
//
// Allowlist-only: only known tracking parameters are removed. Everything else
// survives by construction, so signed and token URLs (token=, signature=,
// key=, code=, ...) are never touched. Applies at use (open/copy), never to
// the stored message.

const TRACKING_PARAMS = new Set([
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_term',
  'utm_content',
  'utm_id',
  'utm_source_platform',
  'utm_creative_format',
  'utm_marketing_tactic',
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

// stripTrackingParams removes known tracking parameters from an http(s) URL.
// Non-URL input, non-http(s) schemes, and URLs without tracking parameters are
// returned unchanged with stripped: false. Never throws.
export function stripTrackingParams(input: string): StripResult {
  const unchanged: StripResult = { url: input, stripped: false, removed: [] }
  if (!input || !/^https?:\/\//i.test(input)) return unchanged
  let parsed: URL
  try {
    parsed = new URL(input)
  } catch {
    return unchanged
  }
  const removed: string[] = []
  const kept: [string, string][] = []
  for (const [name, value] of parsed.searchParams.entries()) {
    if (TRACKING_PARAMS.has(name.toLowerCase())) {
      removed.push(name)
    } else {
      kept.push([name, value])
    }
  }
  if (removed.length === 0) return unchanged
  parsed.search = ''
  for (const [name, value] of kept) {
    parsed.searchParams.append(name, value)
  }
  return { url: parsed.toString(), stripped: true, removed }
}
