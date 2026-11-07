import type { message as messageModels } from '../../../../wailsjs/go/models'

const MAX_CACHED_THREADS = 6
const MAX_CACHED_BYTES = 4 * 1024 * 1024

type CacheEntry = {
  conversation: messageModels.Conversation
  bytes: number
}

const cache = new Map<string, CacheEntry>()
let cachedBytes = 0

function snapshotKey(accountId: string | null, folderId: string | null, threadId: string): string {
  return JSON.stringify([accountId || '', folderId || '', threadId])
}

function addStringBytes(total: { value: number }, value: string | null | undefined) {
  total.value += (value?.length ?? 0) * 2
}

function estimateConversationBytes(conversation: messageModels.Conversation): number {
  const total = { value: 0 }
  addStringBytes(total, conversation.subject)
  addStringBytes(total, conversation.snippet)
  for (const participant of conversation.participants ?? []) {
    addStringBytes(total, participant.name)
    addStringBytes(total, participant.email)
  }
  for (const message of conversation.messages ?? []) {
    addStringBytes(total, message.subject)
    addStringBytes(total, message.snippet)
    addStringBytes(total, message.bodyText)
    addStringBytes(total, message.bodyHtml)
    addStringBytes(total, message.fromName)
    addStringBytes(total, message.fromEmail)
    addStringBytes(total, message.toList)
    addStringBytes(total, message.ccList)
    addStringBytes(total, message.bccList)
    addStringBytes(total, message.replyTo)
  }
  return total.value
}

function touch(key: string, entry: CacheEntry) {
  cache.delete(key)
  cache.set(key, entry)
}

function evictOverflow() {
  while (cache.size > MAX_CACHED_THREADS || cachedBytes > MAX_CACHED_BYTES) {
    const oldest = cache.keys().next().value
    if (oldest === undefined) break
    const entry = cache.get(oldest)
    cache.delete(oldest)
    if (entry) cachedBytes -= entry.bytes
  }
}

/** Returns the most recent bounded snapshot for a thread, or null. */
export function getThreadSnapshot(accountId: string | null, folderId: string | null, threadId: string): messageModels.Conversation | null {
  if (!threadId) return null
  const key = snapshotKey(accountId, folderId, threadId)
  const entry = cache.get(key)
  if (!entry) return null
  touch(key, entry)
  return entry.conversation
}

/** Stores a bounded snapshot for instant remounts; oversized threads are not cached. */
export function setThreadSnapshot(accountId: string | null, folderId: string | null, threadId: string, conversation: messageModels.Conversation): void {
  if (!threadId || !conversation) return
  const bytes = estimateConversationBytes(conversation)
  if (bytes > MAX_CACHED_BYTES) return
  const key = snapshotKey(accountId, folderId, threadId)
  const previous = cache.get(key)
  if (previous) cachedBytes -= previous.bytes
  cache.delete(key)
  cache.set(key, { conversation, bytes })
  cachedBytes += bytes
  evictOverflow()
}

/** Test hook: clears all retained snapshots. */
export function clearThreadSnapshots(): void {
  cache.clear()
  cachedBytes = 0
}
