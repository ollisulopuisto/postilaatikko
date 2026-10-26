/**
 * Decisions behind the conversation timeline (UX-008): which messages are split
 * into new text and quoted history, how the split is shown, and where a thread
 * opens. Pure functions, so they are tested without a DOM; the Svelte components
 * only render what these return.
 *
 * One rule runs through all of it: uncertainty never hides text. Anything that
 * is not clearly quoted history or a signature stays visible, and any outcome
 * that would show nothing falls back to the original message.
 */

export interface ReplyBlockLike {
  id: string
  kind: string
  format: string
  content: string
}

export interface ReplyLike {
  state?: string
  result?: {
    status?: string
    blocks?: ReplyBlockLike[] | null
  } | null
}

export interface ExtractableMessage {
  bodyFetched?: boolean
  bodyHtml?: string
  bodyText?: string
  hasSMIME?: boolean
  hasPGP?: boolean
}

export interface PlanBlock {
  id: string
  kind: string
  format: 'html' | 'text'
  content: string
  /** Folded behind a button until the reader asks for it. */
  collapsed: boolean
  /** A short plain-text hint of what is folded. */
  preview: string
}

export type BodyPlan =
  | { mode: 'original'; reason: 'unavailable' | 'fallback' | 'error' }
  | { mode: 'blocks'; blocks: PlanBlock[]; partial: boolean }

const FOLDABLE_KINDS = new Set(['quoted', 'signature'])

/**
 * A message is split only when its body is stored and readable as it is. A
 * header-only message has nothing to split, and an encrypted one is analyzed
 * from its decrypted view by the existing renderer, not from the stored body.
 */
export function canExtract(message: ExtractableMessage): boolean {
  if (message.hasSMIME || message.hasPGP) return false
  const hasBody = (message.bodyHtml ?? '').trim() !== '' || (message.bodyText ?? '').trim() !== ''
  return hasBody
}

/** Turns an extraction into what the reader shows for one message. */
export function buildBodyPlan(reply: ReplyLike | null | undefined): BodyPlan {
  const result = reply?.result
  if (!result) return { mode: 'original', reason: 'error' }
  if (result.status === 'unavailable') return { mode: 'original', reason: 'unavailable' }
  if (result.status === 'fallback') return { mode: 'original', reason: 'fallback' }

  const blocks: PlanBlock[] = []
  for (const block of result.blocks ?? []) {
    if (!block || (block.content ?? '').trim() === '') continue
    const format = block.format === 'html' ? 'html' : 'text'
    blocks.push({
      id: block.id,
      kind: block.kind,
      format,
      content: block.content,
      collapsed: FOLDABLE_KINDS.has(block.kind),
      preview: textPreview(block.content, format),
    })
  }
  // A split that leaves nothing on screen is not a split.
  if (!blocks.some((block) => !block.collapsed)) return { mode: 'original', reason: 'fallback' }
  return { mode: 'blocks', blocks, partial: result.status === 'partial' }
}

const ENTITIES: Record<string, string> = { '&lt;': '<', '&gt;': '>', '&amp;': '&', '&quot;': '"', '&#39;': "'", '&nbsp;': ' ' }

/** Elements whose content is not text a reader sees. */
const HIDDEN_ELEMENTS = new Set(['script', 'style'])
/** Inline elements do not separate words; every other tag does. */
const INLINE_ELEMENTS = new Set(['a', 'b', 'i', 'u', 'em', 'strong', 'span', 'font', 'code', 'small', 'sub', 'sup'])

const isLetter = (ch: string | undefined) => ch !== undefined && /[a-z]/i.test(ch)

/**
 * The text of an HTML fragment, read one character at a time rather than with
 * tag-shaped patterns: a '<' starts a tag only when a name or '/' follows, a tag
 * runs to the next '>' (or to the end when unterminated), and nothing that was
 * part of a tag can reach the output. The result is shown as text, never as HTML.
 */
function htmlToText(html: string): string {
  let out = ''
  let i = 0
  while (i < html.length) {
    const ch = html[i]
    if (ch !== '<' || !(isLetter(html[i + 1]) || html[i + 1] === '/')) {
      out += ch
      i++
      continue
    }
    const end = html.indexOf('>', i)
    const tagEnd = end === -1 ? html.length : end + 1
    const name = /^<\/?([a-z][a-z0-9]*)/i.exec(html.slice(i, tagEnd))?.[1]?.toLowerCase() ?? ''
    const closing = html[i + 1] === '/'
    i = tagEnd
    if (!closing && HIDDEN_ELEMENTS.has(name)) {
      // Skip to the end of the element; without one, to the end of the text.
      const close = html.toLowerCase().indexOf(`</${name}`, i)
      if (close === -1) break
      const closeEnd = html.indexOf('>', close)
      i = closeEnd === -1 ? html.length : closeEnd + 1
      out += ' '
      continue
    }
    if (!INLINE_ELEMENTS.has(name)) out += ' '
  }
  return out
}

/** A short plain-text preview of a block, for the button that unfolds it. */
export function textPreview(content: string, format: string, max = 90): string {
  let text = content
  if (format === 'html') {
    text = htmlToText(text).replace(/&(?:lt|gt|amp|quot|nbsp|#39);/g, (entity) => ENTITIES[entity] ?? entity)
  }
  text = text.replace(/\s+/g, ' ').trim()
  if (text.length <= max) return text
  return text.slice(0, max - 1).trimEnd() + '\u2026'
}

/** The message a thread opens at: the first unread one, else the newest. */
export function initialTimelineTarget(
  messages: ReadonlyArray<{ id: string; isRead: boolean }> | null | undefined,
): string | null {
  if (!messages || messages.length === 0) return null
  return (messages.find((message) => !message.isRead) ?? messages[messages.length - 1]).id
}

/** Scroll position that puts a card near the top but leaves a peek of the one before it. */
export function timelineScrollTop(targetOffsetTop: number, peek = 48): number {
  return Math.max(0, targetOffsetTop - peek)
}
