import { initialTimelineTarget, timelineScrollTop } from './replyContent'

/** When, after opening, the position is corrected as bodies above it finish loading. */
const SETTLE_DELAYS_MS = [120, 350, 900]

/** Events that mean the reader took over scrolling. */
const TAKEOVER_EVENTS = ['wheel', 'touchstart', 'keydown', 'pointerdown'] as const

/**
 * Opens a thread at its first unread message, else its newest, keeping a peek
 * of the message before it. Message bodies above the target keep growing for a
 * moment after the first paint and the webview does not anchor the scroll
 * position, so the position is corrected a few times, and never after the
 * reader scrolls or leaves the thread. Without a matching card the thread opens
 * at the bottom, as it always did.
 */
export function openTimelineAtTarget(
  container: HTMLElement,
  messages: ReadonlyArray<{ id: string; isRead: boolean }> | null | undefined,
  isCurrent: () => boolean,
  delays: readonly number[] = SETTLE_DELAYS_MS,
): void {
  const targetId = initialTimelineTarget(messages)

  const scrollToTarget = () => {
    const card = targetId
      ? container.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(targetId)}"]`)
      : null
    if (!card) {
      container.scrollTop = container.scrollHeight
      return
    }
    const offset = card.getBoundingClientRect().top - container.getBoundingClientRect().top + container.scrollTop
    container.scrollTop = timelineScrollTop(offset)
  }

  let takenOver = false
  const takeOver = () => {
    takenOver = true
  }
  const stopListening = () => {
    for (const type of TAKEOVER_EVENTS) container.removeEventListener(type, takeOver)
  }
  for (const type of TAKEOVER_EVENTS) container.addEventListener(type, takeOver, { passive: true })

  scrollToTarget()

  delays.forEach((delay, index) => {
    setTimeout(() => {
      if (!takenOver && isCurrent()) scrollToTarget()
      if (index === delays.length - 1) stopListening()
    }, delay)
  })
}
