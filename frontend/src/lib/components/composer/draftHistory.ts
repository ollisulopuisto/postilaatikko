// Kuulas-style branching undo for the composer, backed by the Go
// drafthistory store (SQLite). Every accepted edit is committed as its own
// node - commits are serialized through a queue and never coalesced or
// dropped. Undo/redo move a pointer; editing after undo forks a branch and
// redo then follows the branch most recently descended into.
//
// A commit is acknowledged by the backend before the next one starts, so
// the on-screen state at any moment is at most one in-flight commit ahead
// of durable storage. That in-flight window is the honest crash-loss
// boundary (force-kill mid-write); it is NOT claimed as zero.

// The backend is injected so the same controller works in the main window
// (App bindings) and the detached composer (ComposerApp bindings).
export interface HistoryBackend {
  commit: (draftKey: string, parentId: string, label: string, payload: string) => Promise<string>
  tree: (draftKey: string) => Promise<HistoryTree>
  state: (nodeId: string) => Promise<string>
  checkout: (draftKey: string, nodeId: string) => Promise<void>
  clear: (draftKey: string) => Promise<void>
  move: (oldKey: string, newKey: string) => Promise<void>
}

export interface HistoryNode {
  nodeId: string
  parentId: string
  preferredChildId: string
  label: string
  createdAt: string
}

export interface HistoryTree {
  nodes: HistoryNode[]
  currentNodeId: string
}

export type SnapshotProvider = () => string // canonical JSON of the whole composer state
export type SnapshotApplier = (payload: string) => Promise<void> | void

export class DraftHistoryController {
  private currentNodeId = ''
  private applying = false
  private queue: Promise<unknown> = Promise.resolve()
  private lastPayload = ''
  private treeCache: HistoryTree | null = null

  constructor(
    private draftKey: string,
    private backend: HistoryBackend,
    private snapshot: SnapshotProvider,
    private apply: SnapshotApplier,
    private onError: (err: unknown) => void,
  ) {}

  /** Start over under a new key with no nodes (account switch). */
  reset(newKey: string): void {
    this.draftKey = newKey
    this.currentNodeId = ''
    this.lastPayload = ''
    this.treeCache = null
  }

  /** Re-key the whole tree (unsaved draft got a real draft ID). */
  moveKey(newKey: string): void {
    if (newKey === this.draftKey) return
    const oldKey = this.draftKey
    this.draftKey = newKey // optimistic, like commit payloads
    this.queue = this.queue
      .then(() => this.backend.move(oldKey, newKey))
      .catch((err) => this.onError(err))
  }

  /** Delete the whole tree (actual send or explicit discard). */
  clear(): void {
    const key = this.draftKey
    this.queue = this.queue
      .then(() => this.backend.clear(key))
      .catch((err) => this.onError(err))
  }

  get currentNode(): string {
    return this.currentNodeId
  }

  /** True while a checkout is being applied to the editor; commits are ignored then. */
  get isApplying(): boolean {
    return this.applying
  }

  /** Load an existing tree (draft reopened after restart). */
  async restore(): Promise<void> {
    const tree = await this.backend.tree(this.draftKey)
    this.treeCache = tree
    this.currentNodeId = tree.currentNodeId || ''
    if (this.currentNodeId) {
      this.lastPayload = await this.backend.state(this.currentNodeId)
    }
  }

  /** Commit one accepted edit. No-op when the state is unchanged. */
  commit(label: string): void {
    if (this.applying) return
    const payload = this.snapshot()
    if (payload === this.lastPayload) return
    this.lastPayload = payload // optimistic: identical rapid edits collapse only exact duplicates
    this.queue = this.queue
      .then(() => this.backend.commit(this.draftKey, this.currentNodeId, label, payload))
      .then((nodeId) => {
        this.currentNodeId = nodeId
        this.treeCache = null
      })
      .catch((err) => this.onError(err))
  }

  async tree(): Promise<HistoryTree> {
    if (!this.treeCache) {
      this.treeCache = await this.backend.tree(this.draftKey)
    }
    return this.treeCache
  }

  private async checkout(nodeId: string): Promise<void> {
    await this.queue // land every accepted edit before moving the pointer
    this.applying = true
    try {
      await this.backend.checkout(this.draftKey, nodeId)
      const payload = await this.backend.state(nodeId)
      this.currentNodeId = nodeId
      this.lastPayload = payload
      this.treeCache = null
      await this.apply(payload)
    } finally {
      this.applying = false
    }
  }

  private node(id: string): HistoryNode | undefined {
    return this.treeCache?.nodes.find((n) => n.nodeId === id)
  }

  async undo(): Promise<boolean> {
    const tree = await this.tree()
    const cur = tree.nodes.find((n) => n.nodeId === this.currentNodeId)
    if (!cur || !cur.parentId) return false
    await this.checkout(cur.parentId)
    return true
  }

  async redo(): Promise<boolean> {
    const tree = await this.tree()
    const cur = tree.nodes.find((n) => n.nodeId === this.currentNodeId)
    const next = cur?.preferredChildId || tree.nodes.find((n) => n.parentId === this.currentNodeId)?.nodeId
    if (!next) return false
    await this.checkout(next)
    return true
  }

  /** Jump to any node (branch navigation UI). */
  async goto(nodeId: string): Promise<void> {
    if (nodeId !== this.currentNodeId) await this.checkout(nodeId)
  }

  /** Branch tips: nodes with no children, newest first. */
  async branchTips(): Promise<HistoryNode[]> {
    const tree = await this.tree()
    const parents = new Set(tree.nodes.map((n) => n.parentId).filter(Boolean))
    return tree.nodes.filter((n) => !parents.has(n.nodeId)).reverse()
  }
}
