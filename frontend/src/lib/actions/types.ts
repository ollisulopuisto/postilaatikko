// Shared types for the action layer (docs/plans/2026-10-25-contextual-toolbars-design.md §5.1).
// Type-only: nothing here may import runtime code, so the pure modules next to
// it stay loadable from node tests.

/** Where a list action's targets came from. */
export type ListTargetKind = 'checked' | 'focused' | 'none'

/** The messages a list action applies to, plus the star value those messages imply. */
export interface ListTarget {
  kind: ListTargetKind
  /** Local MESSAGE ids (never thread ids). Empty when kind is 'none'. */
  messageIds: string[]
  /** What the star action should set: true stars them, false unstars them. */
  star: boolean
}

/** Reads the message list's current selection state. */
export interface ListTargetSource {
  hasChecked(): boolean
  checkedIds(): string[]
  /** The keyboard-focused / selected row's message ids. */
  focusedIds(): string[]
  /** True when at least one checked message is not starred. */
  checkedHasUnstarred(): boolean
  /** True when the focused row is starred. */
  selectedStarred(): boolean
}

/** Everything an action needs to decide availability and run. */
export interface ListActionContext {
  target: ListTarget
}

/**
 * The mutations a list action can perform. The app supplies the implementation
 * (toasts, undo, in-flight guards live there); the layer decides which ids and
 * which action.
 */
export interface MessageActionExecutor {
  archive(messageIds: string[]): unknown
  spam(messageIds: string[]): unknown
  markRead(messageIds: string[]): unknown
  markUnread(messageIds: string[]): unknown
  toggleStar(messageIds: string[], star: boolean): unknown
  trash(messageIds: string[], permanent: boolean): unknown
}

export interface RunOptions {
  /** Delete permanently instead of moving to Trash (trash action only). */
  permanent?: boolean
}

/**
 * One action. Label, icon and key hints come from the shortcut registry by
 * `id`, so they are not repeated here.
 */
export interface ActionDef {
  /** Shortcut-registry id, e.g. 'mail.archive'. */
  id: string
  isAvailable(ctx: ListActionContext): boolean
  run(ctx: ListActionContext, executor: MessageActionExecutor, options?: RunOptions): unknown
}
