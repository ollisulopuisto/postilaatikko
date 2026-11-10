// Pure snooze deadline helpers. No imports, so node tests can load this file directly.

type SnoozePresetId = 'laterToday' | 'thisEvening' | 'tomorrow' | 'weekend' | 'nextWeek'

interface SnoozePreset {
  id: SnoozePresetId
  wakeAt: Date
}

/** Morning hour (local) used by the day-based presets. */
const MORNING_HOUR = 8
/** Evening hour (local) for "This evening". */
const EVENING_HOUR = 18
const MIN_LEAD_MS = 60 * 1000
const MAX_LEAD_MS = 3650 * 24 * 60 * 60 * 1000

function atHour(base: Date, dayOffset: number, hour: number): Date {
  const d = new Date(base.getFullYear(), base.getMonth(), base.getDate() + dayOffset, hour, 0, 0, 0)
  return d
}

/** Next quarter hour at least `hours` from `now`. */
function roundedAhead(now: Date, hours: number): Date {
  const quarter = 15 * 60 * 1000
  return new Date(Math.ceil((now.getTime() + hours * 3600 * 1000) / quarter) * quarter)
}

/**
 * Presets offered for `now`, in display order. All times are local. Presets
 * that would repeat an earlier one (or land in the past) are left out.
 */
export function snoozePresets(now: Date): SnoozePreset[] {
  const out: SnoozePreset[] = []
  const seen = new Set<number>()
  const add = (id: SnoozePresetId, wakeAt: Date) => {
    if (wakeAt.getTime() - now.getTime() < MIN_LEAD_MS) return
    if (seen.has(wakeAt.getTime())) return
    seen.add(wakeAt.getTime())
    out.push({ id, wakeAt })
  }
  const later = roundedAhead(now, 3)
  // "Later today" only makes sense while it is still today.
  if (later.getDate() === now.getDate()) add('laterToday', later)
  // "This evening" needs two hours of lead and must differ from "Later today".
  const evening = atHour(now, 0, EVENING_HOUR)
  if (evening.getTime() - now.getTime() >= 2 * 3600 * 1000) add('thisEvening', evening)
  add('tomorrow', atHour(now, 1, MORNING_HOUR))
  // Upcoming Saturday (a Saturday today means next week's Saturday).
  const toSaturday = ((6 - now.getDay() + 7) % 7) || 7
  add('weekend', atHour(now, toSaturday, MORNING_HOUR))
  // Upcoming Monday (a Monday today means next week's Monday).
  const toMonday = ((1 - now.getDay() + 7) % 7) || 7
  add('nextWeek', atHour(now, toMonday, MORNING_HOUR))
  return out
}

type CustomSnoozeResult =
  | { ok: true; wakeAt: Date }
  | { ok: false; reason: 'invalid' | 'past' | 'toofar' }

/** Parses the value of a `datetime-local` input (local wall clock, `YYYY-MM-DDTHH:MM`). */
export function parseCustomSnooze(value: string, now: Date): CustomSnoozeResult {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value.trim())
  if (!m) return { ok: false, reason: 'invalid' }
  const [y, mo, d, h, mi] = m.slice(1).map(Number)
  const wakeAt = new Date(y, mo - 1, d, h, mi, 0, 0)
  // Reject rolled-over dates such as February 31st.
  if (wakeAt.getFullYear() !== y || wakeAt.getMonth() !== mo - 1 || wakeAt.getDate() !== d || wakeAt.getHours() !== h || wakeAt.getMinutes() !== mi) {
    return { ok: false, reason: 'invalid' }
  }
  const lead = wakeAt.getTime() - now.getTime()
  if (lead < MIN_LEAD_MS) return { ok: false, reason: 'past' }
  if (lead > MAX_LEAD_MS) return { ok: false, reason: 'toofar' }
  return { ok: true, wakeAt }
}

/** `datetime-local` value for a Date, in local time. */
export function toDatetimeLocal(date: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}T${p(date.getHours())}:${p(date.getMinutes())}`
}
