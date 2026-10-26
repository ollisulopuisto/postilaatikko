import { initialTimelineTarget, timelineScrollTop } from './replyContent'

/** When, after a change, the position is corrected as bodies finish loading. */
const SETTLE_DELAYS_MS = [120, 350, 900]

/** Events that mean the reader took over scrolling. */
const TAKEOVER_EVENTS = ['wheel', 'touchstart', 'keydown', 'pointerdown'] as const

/** A card that must stay where it is on screen: its id and top, relative to the view. */
export interface ScrollAnchor {
  id: string
  offset: number
}

function findCard(container: HTMLElement, id: string): HTMLElement | null {
  return container.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(id)}"]`)
}

/**
 * Applies a scroll position now and again a few times while message bodies
 * finish loading. The webview does not anchor the scroll position by itself, so
 * without this a change above the view moves what the reader is looking at.
 * Correction stops as soon as the reader scrolls, and for a view they left.
 */
function settle(
  container: HTMLElement,
  apply: () => void,
  isCurrent: () => boolean,
  delays: readonly number[],
): void {
  let takenOver = false
  const takeOver = () => {
    takenOver = true
  }
  const stopListening = () => {
    for (const type of TAKEOVER_EVENTS) container.removeEventListener(type, takeOver)
  }
  for (const type of TAKEOVER_EVENTS) container.addEventListener(type, takeOver, { passive: true })

  apply()

  delays.forEach((delay, index) => {
    setTimeout(() => {
      if (!takenOver && isCurrent()) apply()
      if (index === delays.length - 1) stopListening()
    }, delay)
  })
}

/**
 * Opens a thread at its first unread message, else its newest, keeping a peek
 * of the message before it. Without a matching card the thread opens at the
 * bottom, as it always did.
 */
export function openTimelineAtTarget(
  container: HTMLElement,
  messages: ReadonlyArray<{ id: string; isRead: boolean }> | null | undefined,
  isCurrent: () => boolean,
  delays: readonly number[] = SETTLE_DELAYS_MS,
): void {
  const targetId = initialTimelineTarget(messages)
  settle(
    container,
    () => {
      const card = targetId ? findCard(container, targetId) : null
      if (!card) {
        container.scrollTop = container.scrollHeight
        return
      }
      const offset = card.getBoundingClientRect().top - container.getBoundingClientRect().top + container.scrollTop
      container.scrollTop = timelineScrollTop(offset)
    },
    isCurrent,
    delays,
  )
}

/** The card at the top of the view and where its top is, or null when none is in view. */
export function captureScrollAnchor(container: HTMLElement, messageIds: readonly string[]): ScrollAnchor | null {
  const containerTop = container.getBoundingClientRect().top
  for (const id of messageIds) {
    const card = findCard(container, id)
    if (!card) continue
    const rect = card.getBoundingClientRect()
    if (rect.bottom > containerTop) return { id, offset: rect.top - containerTop }
  }
  return null
}

/** Keeps the anchor card where it was on screen while the content around it changes height. */
export function holdScrollAnchor(
  container: HTMLElement,
  anchor: ScrollAnchor,
  isCurrent: () => boolean,
  delays: readonly number[] = SETTLE_DELAYS_MS,
): void {
  settle(
    container,
    () => {
      const card = findCard(container, anchor.id)
      if (!card) return
      const top = card.getBoundingClientRect().top - container.getBoundingClientRect().top
      container.scrollTop += top - anchor.offset
    },
    isCurrent,
    delays,
  )
}
