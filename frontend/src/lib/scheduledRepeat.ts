// Pure helpers for repeating Send Later and mailing-list recipients. Kept free
// of Svelte and Wails imports so node tests can load them directly.

export type RepeatUnit = 'day' | 'week' | 'month'
export type RepeatEnds = 'never' | 'count' | 'until'
export type ListMode = 'off' | 'cc' | 'bcc'

export interface RepeatForm {
  enabled: boolean
  unit: RepeatUnit
  every: number
  ends: RepeatEnds
  count: number
  until: string // local date, YYYY-MM-DD
}

export interface RepeatRule {
  unit: string
  every: number
  zone: string
  count?: number
  until?: string
}

export interface Addressee {
  name: string
  address: string
}

// The desktop bridge rejects null arguments, so "does not repeat" is a rule
// with an empty unit.
export const NO_REPEAT: RepeatRule = { unit: '', every: 0, zone: '' }

export const defaultRepeatForm = (): RepeatForm => ({
  enabled: false, unit: 'month', every: 1, ends: 'never', count: 12, until: '',
})

/** Returns an i18n error key (without the sendLater. prefix) or '' when valid. */
export function repeatError(form: RepeatForm, firstRun: Date): string {
  if (!form.enabled) return ''
  if (!Number.isInteger(form.every) || form.every < 1 || form.every > 366) return 'repeatEveryInvalid'
  if (form.ends === 'count' && (!Number.isInteger(form.count) || form.count < 2 || form.count > 1000)) return 'repeatCountInvalid'
  if (form.ends === 'until') {
    const end = endOfLocalDay(form.until)
    if (!end || end.getTime() <= firstRun.getTime()) return 'repeatUntilInvalid'
  }
  return ''
}

function endOfLocalDay(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) return null
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), 23, 59, 59)
  return Number.isFinite(date.getTime()) ? date : null
}

export function buildRepeat(form: RepeatForm, zone: string): RepeatRule {
  if (!form.enabled) return { ...NO_REPEAT }
  const rule: RepeatRule = { unit: form.unit, every: form.every, zone }
  if (form.ends === 'count') rule.count = form.count
  if (form.ends === 'until') {
    const end = endOfLocalDay(form.until)
    if (end) rule.until = end.toISOString()
  }
  return rule
}

/** Splits a stored "unit:every" summary. */
export function parseRepeatSummary(value: string | undefined): { unit: RepeatUnit; every: number } | null {
  const match = /^(day|week|month):(\d{1,3})$/.exec(value || '')
  return match ? { unit: match[1] as RepeatUnit, every: Number(match[2]) } : null
}

/**
 * Mailing-list mode: To is the sender, everyone else goes to Cc or Bcc, and the
 * message stays identical for every recipient. Duplicates are removed by
 * address, ignoring case; the sender is never listed twice.
 */
export function applyListMode<T extends { to: Addressee[]; cc: Addressee[]; bcc: Addressee[] }>(
  message: T, mode: ListMode, self: Addressee,
): T {
  if (mode === 'off') return message
  const seen = new Set<string>([self.address.trim().toLowerCase()])
  const others: Addressee[] = []
  for (const person of [...message.to, ...message.cc, ...message.bcc]) {
    const key = (person.address || '').trim().toLowerCase()
    if (!key || seen.has(key)) continue
    seen.add(key)
    others.push(person)
  }
  return {
    ...message,
    to: [self],
    cc: mode === 'cc' ? others : [],
    bcc: mode === 'bcc' ? others : [],
  }
}
