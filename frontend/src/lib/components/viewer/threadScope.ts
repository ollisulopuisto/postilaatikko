type ThreadScopedMessage = { matched?: boolean }

// preserveThreadMessageScope keeps the API's view-scope marker when a body
// fetch replaces a thread context message with its local message model.
export function preserveThreadMessageScope<T extends ThreadScopedMessage>(original: T, fetched: T): T {
  return { ...fetched, matched: original.matched }
}
