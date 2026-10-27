export interface BubbleMessage { id: string; fromEmail: string }

/** Normalize envelope and display-address forms for presentation matching only. */
export function senderAddress(value: string): string {
  const text = (value || '').trim()
  return (text.match(/<\s*([^<>\s]+@[^<>\s]+)\s*>/)?.[1] || text).replace(/^mailto:/i, '').normalize('NFC').toLowerCase()
}

/** Configured identities stay right. Unknown historical threads use their first
 * speaker as a stable visual anchor; extra peer aliases must not make every
 * bubble left-aligned. This fallback is presentation, never identity proof.
 */
export function bubbleLayout(messages: BubbleMessage[], isSelf: (email: string) => boolean) {
  const participants = [...new Set(messages.map(message => senderAddress(message.fromEmail)))]
  const hasSelf = participants.some(email => !!email && isSelf(email))
  const anchor = !hasSelf && participants.length > 1 ? participants[0] : null
  return new Map(messages.map((message, index) => {
    const sender = senderAddress(message.fromEmail)
    return [message.id, {
      side: isSelf(sender) || sender === anchor ? 'right' : 'left',
      showSender: index === 0 || senderAddress(messages[index - 1].fromEmail) !== sender,
    }] as const
  }))
}

/** Double-clicking a control in a row must not spawn another reader. */
export function isRowOpenTarget(target: EventTarget | null, row: EventTarget | null): boolean {
  return target instanceof Element && target.closest('button, a, input, [role="checkbox"], [data-row-action]') === null && row instanceof Element
}
