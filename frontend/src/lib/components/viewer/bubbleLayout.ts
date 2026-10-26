export interface BubbleMessage { id: string; fromEmail: string }

/** Self stays right across aliases; a two-person thread without self uses stable sides. */
export function bubbleLayout(messages: BubbleMessage[], isSelf: (email: string) => boolean) {
  const participants = [...new Set(messages.map(message => message.fromEmail.trim().toLowerCase()))]
  const hasSelf = participants.some(isSelf)
  return new Map(messages.map((message, index) => {
    const sender = message.fromEmail.trim().toLowerCase()
    return [message.id, {
      side: isSelf(sender) || (!hasSelf && participants.length === 2 && sender === participants[1]) ? 'right' : 'left',
      showSender: index === 0 || messages[index - 1].fromEmail.trim().toLowerCase() !== sender,
    }] as const
  }))
}

/** Double-clicking a control in a row must not spawn another reader. */
export function isRowOpenTarget(target: EventTarget | null, row: EventTarget | null): boolean {
  return target instanceof Element && target.closest('button, a, input, [role="checkbox"], [data-row-action]') === null && row instanceof Element
}
