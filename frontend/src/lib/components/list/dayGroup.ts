// Chronological inbox day grouping, extracted from MessageList.svelte so the
// regression tests exercise this exact production code instead of a copy.

export interface DayGroup {
  id: string
  label: string
}

export interface DayGroupLabels {
  today: string
  yesterday: string
}

// Cached Intl.DateTimeFormat instance for month grouping. Constructing
// a new Intl.DateTimeFormat per conversation item causes unnecessary CPU
// churn (~114µs per call) during chronological inbox listing; caching
// reduces execution time by ~23x. The cache key includes the active time
// zone: a formatter binds its zone at construction, while the group id uses
// local getFullYear()/getMonth() in the current zone, so a language-only
// cache would format labels in a stale zone after a system time-zone change
// and month-boundary messages would land under the wrong heading. Constructing
// with an explicit timeZone keeps the formatter and the id logic in the same
// zone.
const monthFormatterCache = new Map<string, Intl.DateTimeFormat>()

// resolvedOptions() costs as much as constructing a formatter (~30µs), which
// would defeat the cache, so the zone is re-resolved only when the cheap
// getTimezoneOffset probe (~0.4µs) changes. The probe also fires on DST
// transitions; that just re-resolves to the same zone string and reuses the
// cached formatter. A zone change to another zone with today's exact offset
// is not detected - the known cost of keeping the lookup fast.
let lastProbeOffset = Number.NaN
let activeTimeZone = ''

function currentTimeZone(): string {
  const offset = new Date().getTimezoneOffset()
  if (offset !== lastProbeOffset) {
    lastProbeOffset = offset
    activeTimeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  }
  return activeTimeZone
}

function monthFormatterFor(lang: string): Intl.DateTimeFormat {
  const timeZone = currentTimeZone()
  const key = `${lang}|${timeZone}`
  let formatter = monthFormatterCache.get(key)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(lang, { month: 'long', timeZone })
    monthFormatterCache.set(key, formatter)
  }
  return formatter
}

export function dayGroupFor(dateValue: Date | string | undefined, lang: string, labels: DayGroupLabels): DayGroup {
  const date = dateValue ? new Date(dateValue) : new Date(0)
  const today = new Date()
  const startToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()
  const startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1).getTime()
  if (startDate === startToday) return { id: 'today', label: labels.today }
  if (startDate === yesterday) return { id: 'yesterday', label: labels.yesterday }
  const sameYear = date.getFullYear() === today.getFullYear()
  const month = monthFormatterFor(lang || 'en').format(date)
  const monthLabel = `${month.charAt(0).toUpperCase()}${month.slice(1)}`
  return {
    // Older conversations are grouped by month, not by an unnecessarily
    // dense daily card. The year is shown only when it differs from today.
    id: `month-${date.getFullYear()}-${date.getMonth()}`,
    label: sameYear ? monthLabel : `${monthLabel} ${date.getFullYear()}`,
  }
}
