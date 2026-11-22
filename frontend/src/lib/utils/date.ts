import { format, isToday, isYesterday, isThisWeek, isThisYear } from 'date-fns'
import { get } from 'svelte/store'
import { _ } from '$lib/i18n'

// A missing or unparsable header date (seen while old mail is still syncing)
// must never throw inside a render: date-fns format() raises RangeError on an
// invalid Date, and one bad row would otherwise break the whole message list.
function isValidDate(date: Date): boolean {
  return date instanceof Date && !Number.isNaN(date.getTime())
}

/**
 * Format a date relative to now for message list display
 * - < 1 minute: "just now"
 * - < 1 hour: "Xm"
 * - < 24 hours: "Xh"
 * - Yesterday: "Yesterday"
 * - This week: "Monday", "Tuesday", etc.
 * - This year: "Dec 15"
 * - Older: "Dec 15, 2023"
 */
export function formatRelativeDate(date: Date): string {
  if (!isValidDate(date)) return ''
  const t = get(_)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMinutes = Math.floor(diffMs / (1000 * 60))
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))

  if (diffMinutes < 1) {
    return t('date.justNow')
  }

  if (diffMinutes < 60) {
    return `${diffMinutes}m`
  }

  if (diffHours < 24 && isToday(date)) {
    return `${diffHours}h`
  }

  if (isYesterday(date)) {
    return t('date.yesterday')
  }

  if (isThisWeek(date)) {
    return format(date, 'EEEE')
  }

  if (isThisYear(date)) {
    return format(date, 'MMM d')
  }

  return format(date, 'MMM d, yyyy')
}

/**
 * Format a date for message header display
 * Shows full date and time
 */
export function formatMessageDate(date: Date): string {
  if (!isValidDate(date)) return ''
  const t = get(_)

  if (isToday(date)) {
    return t('date.todayAt', { values: { time: format(date, 'h:mm a') } })
  }

  if (isYesterday(date)) {
    return t('date.yesterdayAt', { values: { time: format(date, 'h:mm a') } })
  }

  if (isThisYear(date)) {
    return format(date, 'MMM d \'at\' h:mm a')
  }

  return format(date, 'MMM d, yyyy \'at\' h:mm a')
}

/**
 * Format a date for full display (tooltips, etc.)
 */
export function formatFullDate(date: Date): string {
  if (!isValidDate(date)) return ''
  return format(date, 'EEEE, MMMM d, yyyy \'at\' h:mm:ss a')
}
