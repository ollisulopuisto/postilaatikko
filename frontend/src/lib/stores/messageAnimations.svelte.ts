export type MessageAnimationKind = 'archive' | 'trash' | 'move' | 'spam'

// Animation duration in milliseconds before DOM removal
export const MESSAGE_EXIT_ANIM_DURATION_MS = 220

class MessageAnimationsStore {
  // Svelte 5 reactive map tracking active animations by message ID
  private animating = $state<Map<string, MessageAnimationKind>>(new Map())

  isAnimating(messageId: string): MessageAnimationKind | undefined {
    return this.animating.get(messageId)
  }

  hasAnimating(messageIds: Iterable<string>): MessageAnimationKind | undefined {
    for (const id of messageIds) {
      const kind = this.animating.get(id)
      if (kind) return kind
    }
    return undefined
  }

  isAnyAnimating(): boolean {
    return this.animating.size > 0
  }

  // Default safety TTL so animations self-retire even in workspaces/tabs without a MessageList
  private static readonly AUTO_EXPIRE_MS = 600

  startRemoval(messageIds: Iterable<string>, kind: MessageAnimationKind, ttlMs: number = MessageAnimationsStore.AUTO_EXPIRE_MS) {
    const ids = [...new Set(messageIds)].filter(Boolean)
    if (ids.length === 0) return

    const next = new Map(this.animating)
    for (const id of ids) {
      next.set(id, kind)
    }
    this.animating = next

    // Ensure the operation has a bounded lifecycle independent of any list component
    if (ttlMs > 0 && typeof setTimeout !== 'undefined') {
      setTimeout(() => {
        this.finishRemoval(ids)
      }, ttlMs)
    }
  }

  finishRemoval(messageIds: Iterable<string>) {
    const next = new Map(this.animating)
    let changed = false
    for (const id of messageIds) {
      if (next.delete(id)) {
        changed = true
      }
    }
    if (changed) {
      this.animating = next
    }
  }

  clear() {
    if (this.animating.size > 0) {
      this.animating = new Map()
    }
  }
}

export const messageAnimations = new MessageAnimationsStore()
