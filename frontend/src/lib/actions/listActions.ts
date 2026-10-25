import type { ActionDef, ListActionContext, ListTarget, MessageActionExecutor, RunOptions } from './types'

// Actions on the message list's target (checked messages or the focused row).
// Labels, icons and key hints live in the shortcut registry under the same id.

const hasTarget = (ctx: ListActionContext): boolean => ctx.target.messageIds.length > 0

export const LIST_ACTIONS: ActionDef[] = [
  { id: 'mail.archive', isAvailable: hasTarget, run: (ctx, x) => x.archive(ctx.target.messageIds) },
  { id: 'mail.spam', isAvailable: hasTarget, run: (ctx, x) => x.spam(ctx.target.messageIds) },
  { id: 'mail.markRead', isAvailable: hasTarget, run: (ctx, x) => x.markRead(ctx.target.messageIds) },
  { id: 'mail.markUnread', isAvailable: hasTarget, run: (ctx, x) => x.markUnread(ctx.target.messageIds) },
  { id: 'mail.star', isAvailable: hasTarget, run: (ctx, x) => x.toggleStar(ctx.target.messageIds, ctx.target.star) },
  { id: 'mail.trash', isAvailable: hasTarget, run: (ctx, x, options) => x.trash(ctx.target.messageIds, options?.permanent === true) },
]

const BY_ID = new Map(LIST_ACTIONS.map((def) => [def.id, def]))

export function getListAction(id: string): ActionDef | undefined {
  return BY_ID.get(id)
}

/**
 * Runs a list action against an already-resolved target. Does nothing for an
 * unknown id or when the action is unavailable (for example no target).
 */
export async function runListAction(
  id: string,
  target: ListTarget,
  executor: MessageActionExecutor,
  options?: RunOptions,
): Promise<void> {
  const def = BY_ID.get(id)
  if (!def) return
  const ctx: ListActionContext = { target }
  if (!def.isAvailable(ctx)) return
  await def.run(ctx, executor, options)
}
