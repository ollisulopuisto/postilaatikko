/**
 * Bounded session cache for inline attachment data URLs. SQLite owns the durable
 * copies; a cache miss is fetched again by the reader. Eviction does not revoke
 * data already held by an open message.
 */
const MAX_MESSAGES = 128
const MAX_STRING_BYTES = 32 * 1024 * 1024

type Entry = { data: Record<string, string>; bytes: number }
const cache = new Map<string, Entry>()
let stringBytes = 0
let generation = 0

export function getCacheGeneration(): number { return generation }

/** Get cached attachments and mark the message as recently used. */
export function getCached(messageId: string): Record<string, string> | null {
  const entry = cache.get(messageId)
  if (!entry) return null
  cache.delete(messageId)
  cache.set(messageId, entry)
  return entry.data
}

/** Store attachments, evicting least-recently-used messages within the budget. */
export function setCache(messageId: string, data: Record<string, string>): void {
  const previous = cache.get(messageId)
  if (previous) {
    stringBytes -= previous.bytes
    cache.delete(messageId)
  }

  // Two bytes per UTF-16 code unit is conservative for the data URL strings.
  // Include keys, but do not claim this bounds JS objects or decoded images.
  let bytes = messageId.length * 2
  for (const [contentId, dataUrl] of Object.entries(data)) {
    bytes += (contentId.length + dataUrl.length) * 2
    if (bytes > MAX_STRING_BYTES) return
  }
  if (bytes > MAX_STRING_BYTES) return

  while (cache.size >= MAX_MESSAGES || stringBytes + bytes > MAX_STRING_BYTES) {
    const oldest = cache.keys().next()
    if (oldest.done) break
    const entry = cache.get(oldest.value)!
    stringBytes -= entry.bytes
    cache.delete(oldest.value)
  }
  cache.set(messageId, { data, bytes })
  stringBytes += bytes
}

/** Clear attachments and invalidate requests started before this reset. */
export function clearCache(): void {
  generation++
  cache.clear()
  stringBytes = 0
}

/** Get the number of messages retained in the session cache. */
export function getCacheSize(): number {
  return cache.size
}
