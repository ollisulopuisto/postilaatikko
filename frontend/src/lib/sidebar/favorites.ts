// Folder favorites for the quick-access rail. A favorite is a pinned folder
// whose navigation entry is also in the rail's quick list, so starring any
// folder in the tree or the All folders browser pins it first and then marks
// it. Unstarring only removes the quick mark; the pin stays so nothing the
// user may have arranged deliberately disappears.

import { sidebarPins } from '$lib/stores/sidebarPins.svelte'
import { navigationCards } from '$lib/stores/navigationCards.svelte'

export const MAX_FAVORITES = 10

export function folderQuickId(accountId: string, folderId: string): string {
  return `folder:${accountId}:${folderId}`
}

export function isFolderFavorite(accountId: string, folderId: string): boolean {
  return navigationCards.state.quick.includes(folderQuickId(accountId, folderId))
}

export function favoritesFull(): boolean {
  return navigationCards.state.quick.length >= MAX_FAVORITES
}

export function toggleFolderFavorite(accountId: string, folder: { id: string; path: string; name: string }): void {
  if (!sidebarPins.has(accountId, folder.id)) {
    sidebarPins.toggle(accountId, { id: folder.id, path: folder.path, name: folder.name })
  }
  navigationCards.toggleQuick(folderQuickId(accountId, folder.id))
}
