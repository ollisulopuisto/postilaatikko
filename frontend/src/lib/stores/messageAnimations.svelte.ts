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

  private holds = new Map<number, number>()
  private generations = new Map<string, number>()
  private nextGeneration = 0

  // Each callback owns only the captured generation, never a newer operation.
  holdActive() {
    const captured = new Map(this.generations)
    for (const generation of captured.values()) this.holds.set(generation, (this.holds.get(generation) || 0) + 1)
    return () => {
      for (const [id, generation] of captured) {
        const remaining = Math.max(0, (this.holds.get(generation) || 1) - 1)
        if (remaining) this.holds.set(generation, remaining)
        if (!remaining) this.holds.delete(generation)
        if (!remaining && this.generations.get(id) === generation) this.finishRemoval([id])
      }
    }
  }

  settleRemoval(messageIds: Iterable<string>) {
    const captured = [...messageIds].map(id => [id, this.generations.get(id)] as const)
    setTimeout(() => {
      for (const [id, generation] of captured) {
        if ((generation === undefined || !this.holds.get(generation)) && this.generations.get(id) === generation) {
          this.finishRemoval([id])
        }
      }
    }, MESSAGE_EXIT_ANIM_DURATION_MS)
  }

  startRemoval(messageIds: Iterable<string>, kind: MessageAnimationKind) {
    const next = new Map(this.animating)
    for (const id of messageIds) {
      if (id) {
        next.set(id, kind)
        this.generations.set(id, ++this.nextGeneration)
      }
    }
    this.animating = next
  }

  finishRemoval(messageIds: Iterable<string>) {
    const next = new Map(this.animating)
    let changed = false
    for (const id of messageIds) {
      if (next.delete(id)) {
        this.generations.delete(id)
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
      this.generations.clear()
      this.holds.clear()
    }
  }
}

export const messageAnimations = new MessageAnimationsStore()
