/**
 * The reply extractor returns HTML whose remote images are already blocked: the
 * source is a placeholder and the real address is kept in data-original-src
 * (internal/email BlockRemoteImages). These helpers let the reader detect that
 * and put the real address back once the user has chosen to load images.
 */

const BLOCKED_IMG = /<img\b[^>]*>/gi
const ORIGINAL_SRC = /\sdata-original-src="(https?:\/\/[^"]*)"/i
const SRC = /\ssrc="[^"]*"/i

/** True when html holds an image the extractor blocked. */
export function hasBlockedRemoteImages(html: string): boolean {
  if (!html) return false
  return /<img\b[^>]*\sdata-original-src="https?:\/\//i.test(html)
}

/**
 * Replaces the placeholder source of every blocked image with its original
 * http(s) address. Images without a web address in data-original-src (cid:,
 * data: and anything else) are left exactly as they are. The address is already
 * HTML-escaped by the extractor, so it is put back as it is.
 */
export function restoreBlockedImages(html: string): string {
  if (!html) return html
  return html.replace(BLOCKED_IMG, (tag) => {
    const original = ORIGINAL_SRC.exec(tag)
    if (!original || !SRC.test(tag)) return tag
    return tag
      .replace(ORIGINAL_SRC, '')
      .replace(SRC, () => ` src="${original[1]}"`)
  })
}
