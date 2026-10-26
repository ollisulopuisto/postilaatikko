// Runtime checks and helpers for the view contract (D-002, UX-002).
//
// types.ts is erased at build time, so a response from the backend or from an
// agent needs a runtime check before the UI trusts it. The guards are strict:
// they reject unknown fields as well as missing ones, matching how the Go
// tests decode the fixtures. No imports at runtime, so node tests can load this file.

import type { Completeness, MessageRef, ThreadRef } from './types'

type Json = Record<string, unknown>

function isRecord(value: unknown): value is Json {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// True when value has every required key, only required or optional keys, and
// each key passes its check.
function isShape(value: unknown, required: Record<string, (v: unknown) => boolean>, optional: Record<string, (v: unknown) => boolean> = {}): boolean {
  if (!isRecord(value)) return false
  for (const key of Object.keys(value)) {
    if (!(key in required) && !(key in optional)) return false
  }
  for (const [key, check] of Object.entries(required)) {
    if (!(key in value) || !check(value[key])) return false
  }
  return Object.entries(optional).every(([key, check]) => !(key in value) || check(value[key]))
}

const isString = (v: unknown): v is string => typeof v === 'string'
const isBool = (v: unknown): v is boolean => typeof v === 'boolean'
const isCount = (v: unknown): v is number => typeof v === 'number' && Number.isInteger(v) && v >= 0
const isTimestamp = (v: unknown): v is string => isString(v) && !Number.isNaN(Date.parse(v))
const arrayOf = (check: (v: unknown) => boolean) => (v: unknown): boolean => Array.isArray(v) && v.every(check)

const CONTENT_STATES = ['cached', 'not_fetched', 'fetch_failed']

const isAddress = (v: unknown): boolean => isShape(v, { name: isString, email: isString })
const isMessageRef = (v: unknown): boolean => isShape(v, { accountId: isString, messageId: isString })
const isThreadRef = (v: unknown): boolean => isShape(v, { accountId: isString, threadId: isString })

const isThreadMessage = (v: unknown): boolean =>
  isShape(
    v,
    {
      ref: isMessageRef,
      folderId: isString,
      from: isAddress,
      date: isTimestamp,
      matched: isBool,
      isRead: isBool,
      isStarred: isBool,
      isAnswered: isBool,
      hasAttachments: isBool,
      contentState: (s) => isString(s) && CONTENT_STATES.includes(s),
    },
    { rfcMessageId: isString },
  )

const isBodyCounts = (v: unknown): boolean =>
  isShape(v, { total: isCount, cached: isCount, notFetched: isCount, fetchFailed: isCount })

const isCompleteness = (v: unknown): boolean => isShape(v, { complete: isBool, bodies: isBodyCounts })

const isThreadSummary = (v: unknown): boolean =>
  isShape(v, {
    ref: isThreadRef,
    subject: isString,
    snippet: isString,
    latestDate: isTimestamp,
    participants: arrayOf(isAddress),
    messageCount: isCount,
    unreadCount: isCount,
    hasAttachments: isBool,
    isStarred: isBool,
    matched: arrayOf(isMessageRef),
  })

const isPartialError = (v: unknown): boolean => isShape(v, { accountId: isString, code: isString, message: isString })

/** True when value is a well-formed opened thread. */
export function isThreadView(value: unknown): boolean {
  return isShape(value, {
    ref: isThreadRef,
    subject: isString,
    latestDate: isTimestamp,
    participants: arrayOf(isAddress),
    messageCount: isCount,
    matchedCount: isCount,
    messages: arrayOf(isThreadMessage),
    completeness: isCompleteness,
  })
}

/** True when value is a well-formed page of thread summaries. */
export function isThreadPage(value: unknown): boolean {
  return isShape(
    value,
    { items: arrayOf(isThreadSummary), complete: isBool },
    { nextCursor: isString, errors: arrayOf(isPartialError) },
  )
}

const SCOPE_KINDS = ['all_inboxes', 'folder', 'virtual_folder', 'search']
const SCOPE_FIELDS = ['accountId', 'folderId', 'virtualFolderId', 'query', 'within']

function has(scope: Json, key: string): boolean {
  const value = scope[key]
  return value !== undefined && value !== ''
}

/**
 * Returns why a scope is invalid, or null when it is valid. Mirrors
 * Scope.Validate in internal/views/model.go; both are checked against
 * internal/views/testdata/scopes.json.
 */
export function scopeError(scope: unknown): string | null {
  if (!isRecord(scope)) return 'scope must be an object'
  const kind = scope.kind
  if (!isString(kind) || !SCOPE_KINDS.includes(kind)) return `unknown scope kind ${String(kind)}`
  for (const key of Object.keys(scope)) {
    if (key !== 'kind' && !SCOPE_FIELDS.includes(key)) return `unknown scope field ${key}`
  }
  const only = (...allowed: string[]): boolean => SCOPE_FIELDS.every((f) => allowed.includes(f) || !has(scope, f))

  switch (kind) {
    case 'all_inboxes':
      return only() ? null : 'all_inboxes scope takes no other fields'
    case 'folder':
      if (!has(scope, 'accountId') || !has(scope, 'folderId')) return 'folder scope needs accountId and folderId'
      return only('accountId', 'folderId') ? null : 'folder scope takes only accountId and folderId'
    case 'virtual_folder':
      if (!has(scope, 'virtualFolderId')) return 'virtual_folder scope needs virtualFolderId'
      return only('virtualFolderId') ? null : 'virtual_folder scope takes only virtualFolderId'
    default: {
      if (!isString(scope.query) || scope.query.trim() === '') return 'search scope needs a query'
      if (!only('query', 'within')) return 'search scope takes only query and within'
      if (scope.within === undefined) return null
      if (isRecord(scope.within) && scope.within.kind === 'search') return 'a search cannot be limited to another search'
      const inner = scopeError(scope.within)
      return inner === null ? null : `search within: ${inner}`
    }
  }
}

/** A string unique per account and message. Keys are local; do not persist them. */
export function messageKey(ref: MessageRef): string {
  return `${encodeURIComponent(ref.accountId)}/${encodeURIComponent(ref.messageId)}`
}

/** A string unique per account and thread. Keys are local; do not persist them. */
export function threadKey(ref: ThreadRef): string {
  return `${encodeURIComponent(ref.accountId)}/${encodeURIComponent(ref.threadId)}`
}

/** The refs of the messages that are in the view's scope. */
export function matchedRefs(messages: { ref: MessageRef; matched: boolean }[]): MessageRef[] {
  return messages.filter((m) => m.matched).map((m) => m.ref)
}

/** Counts body states; complete only when every body is cached. Mirrors CompletenessOf in Go. */
export function completenessOf(messages: { contentState: string }[]): Completeness {
  const bodies = { total: 0, cached: 0, notFetched: 0, fetchFailed: 0 }
  for (const m of messages) {
    bodies.total++
    if (m.contentState === 'cached') bodies.cached++
    else if (m.contentState === 'fetch_failed') bodies.fetchFailed++
    else bodies.notFetched++
  }
  return { complete: bodies.notFetched === 0 && bodies.fetchFailed === 0, bodies }
}
