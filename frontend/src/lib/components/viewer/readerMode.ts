/**
 * How the reader shows a thread. Conversation splits each message into new text
 * and folded history (UX-008); Traditional shows every message in full, as the
 * reader always did. Both show the same thread, flags and selection: the mode
 * only changes how a message body is drawn.
 */
export const READER_MODES = ['conversation', 'traditional'] as const

export type ReaderMode = (typeof READER_MODES)[number]

/** Anything but a known stored value means the default, Conversation. */
export function normalizeReaderMode(value: unknown): ReaderMode {
  return value === 'traditional' ? 'traditional' : 'conversation'
}
