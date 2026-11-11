// Per-folder membership counts for the app-local Read Later / Reply Later
// queues. Backs the folder-list filter in the Kaikki kansiot panel. Counts
// load on demand and refresh whenever queue membership or folder sync state
// changes, mirroring the readLater/replyLater queue stores' event sets.
import { GetQueueFolderCounts } from '../../../wailsjs/go/app/App'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import type { QueueCountsByFilter } from '$lib/sidebar/folderListFilter'

let counts = $state<Required<QueueCountsByFilter>>({ readLater: {}, replyLater: {} })
let subscribed = false
let loading: Promise<void> | null = null

async function load(force = false): Promise<void> {
  if (!subscribed) {
    subscribed = true
    for (const event of ['readLater:changed', 'readLater:refresh', 'readLater:synced', 'replyLater:changed', 'replyLater:refresh', 'replyLater:synced', 'folder:synced']) {
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
  await load(true)
}

export const folderQueueCounts = {
  get counts() { return counts },
  load,
}
