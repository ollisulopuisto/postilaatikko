// Per-folder membership counts for the Read Later / Reply Later queues.
// Backs the folder-list filter in the Kaikki kansiot panel. Counts load on
// demand while a queue filter is active and refresh whenever queue
// membership, message location, or folder sync state changes. Consumers
// hold an acquire() release; nothing reloads while no filter is showing.
import { GetQueueFolderCounts } from '../../../wailsjs/go/app/App'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import type { QueueCountsByFilter } from '$lib/sidebar/folderListFilter'

let counts = $state<Required<QueueCountsByFilter>>({ readLater: {}, replyLater: {} })
let subscribed = false
let activeConsumers = 0
let loading: Promise<void> | null = null

async function load(force = false): Promise<void> {
  if (!subscribed) {
    subscribed = true
    for (const event of ['readLater:changed', 'readLater:refresh', 'readLater:synced', 'replyLater:changed', 'replyLater:refresh', 'replyLater:synced', 'messages:moved', 'messages:deleted', 'folder:synced']) {
      EventsOn(event, () => { void reload() })
    }
  }
  if (loading && !force) return loading
  loading = (async () => {
    try {
      const result = await GetQueueFolderCounts()
      counts = { readLater: result?.readLater ?? {}, replyLater: result?.replyLater ?? {} }
    } catch (err) {
      console.warn('Failed to load queue folder counts:', err)
    } finally {
      loading = null
    }
  })()
  return loading
}

async function reload(): Promise<void> {
  // Queue, move and sync events keep arriving after the filter closes;
  // rescanning then would be wasted work, so only active consumers refresh.
  if (activeConsumers > 0) await load(true)
}

/** Hold while a queue folder filter is visible; release when it closes. */
function acquire(): () => void {
  activeConsumers++
  if (activeConsumers === 1) void load()
  return () => { activeConsumers = Math.max(0, activeConsumers - 1) }
}

export const folderQueueCounts = {
  get counts() { return counts },
  load,
  acquire,
}
