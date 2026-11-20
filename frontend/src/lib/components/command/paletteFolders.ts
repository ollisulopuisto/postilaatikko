interface FolderEntry {
  id: string
  name: string
  path: string
  type: string
  hidden?: boolean
  parentId?: string
}
interface FolderTree { folder?: FolderEntry; children?: FolderTree[] }
interface AccountEntry { account: { id: string; name?: string; email?: string }; folders?: FolderTree[] }
export interface PaletteFolder {
  accountId: string
  folderId: string
  name: string
  path: string
  type: string
  context: string
}

/** Search the already-loaded catalog. No provider requests or mailbox reads. */
export function matchPaletteFolders(accounts: AccountEntry[], catalog: Record<string, FolderEntry[]>, query: string): PaletteFolder[] {
  const q = query.trim().toLocaleLowerCase()
  if (!q) return []
  const results: { row: PaletteFolder; rank: number; order: number }[] = []
  const seen = new Set<string>()
  for (const acc of accounts) {
    const entries: FolderEntry[] = []
    const walk = (trees: FolderTree[]) => {
      for (const tree of trees) {
        if (tree.folder?.hidden) continue
        if (tree.folder) entries.push(tree.folder)
        walk(tree.children || [])
      }
    }
    const collect = () => {
      if (catalog[acc.account.id]) {
        entries.push(...catalog[acc.account.id])
        return
      }
      walk(acc.folders || [])
    }
    collect()
    const byId = new Map(entries.map(f => [f.id, f]))
    const hidden = (folder: FolderEntry) => {
      const visited = new Set<string>()
      let current: FolderEntry | undefined = folder
      while (current) {
        if (current.hidden || visited.has(current.id)) return true
        visited.add(current.id)
        current = current.parentId ? byId.get(current.parentId) : undefined
      }
      return false
    }
    for (const folder of entries) {
      const key = JSON.stringify([acc.account.id, folder.id])
      if (!folder.id || hidden(folder) || seen.has(key)) continue
      seen.add(key)
      const name = folder.name.toLocaleLowerCase()
      const path = folder.path.toLocaleLowerCase()
      if (!name.includes(q) && !path.includes(q)) continue
      const parent = folder.path.includes('/') ? folder.path.slice(0, folder.path.lastIndexOf('/')) : ''
      results.push({ rank: name === q ? 0 : name.startsWith(q) ? 1 : 2, order: results.length, row: {
        accountId: acc.account.id, folderId: folder.id, name: folder.name, path: folder.path, type: folder.type,
        context: [acc.account.name || acc.account.email || acc.account.id, parent].filter(Boolean).join(' · '),
      } })
    }
  }
  return results.sort((a, b) => a.rank - b.rank || a.order - b.order).slice(0, 20).map(item => item.row)
}
