export interface ReactionInfo { emoji?: string; targetMessageId?: string; provider?: string; invalid?: boolean }
export interface ReactionMessage {
  id: string; accountId: string; messageId?: string; fromName?: string; fromEmail?: string
  reaction?: ReactionInfo | null; hasAttachments?: boolean; hasSMIME?: boolean; hasPGP?: boolean
}
export interface ReactionGroup<T> { emoji: string; messages: T[] }

const PROVIDERS = new Set(['gmail', 'notification', 'rfc9078'])
const rfcID = (value = '') => value.trim().replace(/^</, '').replace(/>$/, '')

/** Hide a notification only when its exact, account-scoped target is present.
 * Never guess the previous row. The source message remains in the data model.
 */
export function reactionLayout<T extends ReactionMessage>(messages: T[]) {
  const targets = new Map<string, T>()
  for (const message of messages) {
    if (message.accountId && message.messageId && (!message.reaction || message.reaction.invalid)) {
      const key = `${message.accountId}\0${rfcID(message.messageId)}`
      if (!targets.has(key)) targets.set(key, message)
    }
  }
  const hidden = new Set<string>()
  const groups = new Map<string, ReactionGroup<T>[]>()
  const seen = new Set<string>()
  for (const message of messages) {
    const info = message.reaction
    if (!info || info.invalid || !info.emoji || !info.targetMessageId || !PROVIDERS.has(info.provider || '') || message.hasAttachments || message.hasSMIME || message.hasPGP) continue
    const target = targets.get(`${message.accountId}\0${rfcID(info.targetMessageId)}`)
    if (!target || target.id === message.id) continue
    hidden.add(message.id)
    const duplicateKey = JSON.stringify([message.accountId, message.messageId || message.id, target.id, info.emoji, message.fromEmail])
    if (seen.has(duplicateKey)) continue
    seen.add(duplicateKey)
    const targetGroups = groups.get(target.id) || []
    let group = targetGroups.find(group => group.emoji === info.emoji)
    if (!group) { group = { emoji: info.emoji, messages: [] }; targetGroups.push(group) }
    group.messages.push(message)
    groups.set(target.id, targetGroups)
  }
  return { hidden, groups, messages: messages.filter(message => !hidden.has(message.id)) }
}
