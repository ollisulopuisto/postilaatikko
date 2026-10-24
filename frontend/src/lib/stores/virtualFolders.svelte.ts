import {
  CreateVirtualFolder,
  DeleteVirtualFolder,
  GetVirtualFolders,
  GetVirtualFolderUnreadCounts,
  ReorderVirtualFolders,
  UpdateVirtualFolder,
} from '../../../wailsjs/go/app/App'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import type { virtualfolder } from '../../../wailsjs/go/models'

class VirtualFoldersStore {
  folders = $state<virtualfolder.VirtualFolder[]>([])
  unreadCounts = $state<Record<string, number>>({})
  loading = $state(false)
  private loaded = false
  private eventsInitialized = false
  private refreshTimer: ReturnType<typeof setTimeout> | undefined

  async load(force = false): Promise<void> {
    if (this.loading || (this.loaded && !force)) return
    this.loading = true
    try {
      const folders = await GetVirtualFolders()
      this.folders = folders || []
      this.loaded = true
      await this.refreshCounts()
      this.initEvents()
    } finally {
      this.loading = false
    }
  }

  async refreshCounts(): Promise<void> {
    try {
      this.unreadCounts = (await GetVirtualFolderUnreadCounts()) || {}
    } catch (error) {
      console.error('Failed to refresh virtual folder counts:', error)
    }
  }

  scheduleCountRefresh(): void {
    if (this.refreshTimer) clearTimeout(this.refreshTimer)
    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = undefined
      void this.refreshCounts()
    }, 250)
  }

  async save(folder: virtualfolder.VirtualFolder): Promise<void> {
    if (folder.id) await UpdateVirtualFolder(folder)
    else await CreateVirtualFolder(folder)
    this.loaded = false
    await this.load(true)
  }

  async remove(id: string): Promise<void> {
    await DeleteVirtualFolder(id)
    this.folders = this.folders.filter((item) => item.id !== id)
    const { [id]: _removed, ...remainingCounts } = this.unreadCounts
    this.unreadCounts = remainingCounts
  }

  async move(id: string, delta: -1 | 1): Promise<void> {
    const from = this.folders.findIndex((item) => item.id === id)
    const to = from + delta
    if (from < 0 || to < 0 || to >= this.folders.length) return
    const next = [...this.folders]
    ;[next[from], next[to]] = [next[to], next[from]]
    await ReorderVirtualFolders(next.map((item) => item.id))
    next.forEach((item, orderIndex) => { item.orderIndex = orderIndex })
    this.folders = next
  }

  private initEvents(): void {
    if (this.eventsInitialized) return
    this.eventsInitialized = true
    EventsOn('messages:readChanged', () => this.scheduleCountRefresh())
    EventsOn('folders:countsChanged', () => this.scheduleCountRefresh())
    EventsOn('folder:synced', () => this.scheduleCountRefresh())
    EventsOn('messages:updated', () => this.scheduleCountRefresh())
  }
}

export const virtualFoldersStore = new VirtualFoldersStore()
