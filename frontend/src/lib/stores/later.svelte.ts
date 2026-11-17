// Presentation compatibility layer: no data migration, old clients keep working.
import { readLaterQueue } from './readLater.svelte'
import { replyLaterQueue } from './replyLater.svelte'
import { UndoOperation } from '../../../wailsjs/go/app/App'

export type LaterRef = { accountId: string; conversationId: string; indexed?: boolean }
export type LaterChange = { changed: boolean; operationId: string; operationIds: string[] }
type Ref = LaterRef
const key = (ref: Ref) => JSON.stringify([ref.accountId, ref.conversationId])
let busy = $state<Set<string>>(new Set())

export async function undoLaterChange(result: Pick<LaterChange, 'operationIds'>) {
  // Reverse the two legacy writes in reverse order. Stop on failure so callers
  // retain their receipt rather than silently claiming a complete restoration.
  for (const id of [...result.operationIds].reverse()) await UndoOperation(id)
}

async function load(force = false) {
  await Promise.all([readLaterQueue.load(force), replyLaterQueue.load(force)])
}

async function setWithUndo(accountId: string, conversationId: string, inQueue: boolean): Promise<LaterChange | null> {
  const id = key({ accountId, conversationId })
  if (busy.has(id)) return null
  busy = new Set([...busy, id])
  const result: LaterChange = { changed: false, operationId: '', operationIds: [] }
  let restoreReadWithoutReceipt = false
  try {
    await load()
    const read = readLaterQueue.has(accountId, conversationId)
    const reply = replyLaterQueue.has(accountId, conversationId)
    const remember = (change: { changed: boolean; operationId?: string } | null) => {
      if (!change?.changed) return
      result.changed = true
      if (change.operationId) result.operationIds.push(change.operationId)
      result.operationId = result.operationIds[0] || ''
    }
    if (inQueue) {
      if (!read && !reply) remember(await readLaterQueue.setWithUndo(accountId, conversationId, true))
    } else {
      if (read) {
        const change = await readLaterQueue.setWithUndo(accountId, conversationId, false)
        remember(change)
        restoreReadWithoutReceipt = !!change?.changed && !change.operationId
      }
      if (reply) remember(await replyLaterQueue.set(accountId, conversationId, false))
    }
    return result
  } catch (err) {
    // Preserve a legacy mark if the second provider/helper write fails.
    try {
      await undoLaterChange(result)
      if (restoreReadWithoutReceipt) await readLaterQueue.setWithUndo(accountId, conversationId, true)
    }
    catch (rollbackError) { throw new Error(`${String(err)}; restoration failed: ${String(rollbackError)}`, { cause: rollbackError }) }
    throw err
  } finally {
    busy = new Set([...busy].filter(item => item !== id))
    // Refresh both stores even after a partial/provider failure.
    void load(true).catch(() => {})
  }
}

export const laterQueue = {
  get refs(): Ref[] {
    const refs = new Map<string, Ref>()
    for (const ref of [...readLaterQueue.refs, ...replyLaterQueue.refs]) {
      const previous = refs.get(key(ref))
      refs.set(key(ref), { ...previous, ...ref })
    }
    return [...refs.values()]
  },
  get count() { return this.refs.length },
  get ready() { return readLaterQueue.ready && replyLaterQueue.ready },
  get error() { return replyLaterQueue.error },
  load,
  has(accountId: string, conversationId: string) { return readLaterQueue.has(accountId, conversationId) || replyLaterQueue.has(accountId, conversationId) },
  isBusy(accountId: string, conversationId: string) { return busy.has(key({ accountId, conversationId })) || readLaterQueue.isBusy(accountId, conversationId) || replyLaterQueue.isBusy(accountId, conversationId) },
  setWithUndo,
  set: setWithUndo,
}
