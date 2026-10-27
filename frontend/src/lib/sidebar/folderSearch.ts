// Search over the folder checklist in the virtual folder dialog (P-036).

export interface SearchableFolder {
  id: string
  label: string
}

// Lowercases and composes the text, so Finnish letters match whether the
// input or the label stores them as one character or as base letter plus accent.
function normalizeText(text: string): string {
  return text.normalize('NFC').toLowerCase()
}

// Returns the folders whose label contains every word of the query, in any
// order and position. Case and extra spaces are ignored. A blank query returns
// the input unchanged. The input array is never modified.
export function filterFolders<T extends SearchableFolder>(items: T[], query: string): T[] {
  const words = normalizeText(query).split(/\s+/).filter(word => word !== '')
  if (words.length === 0) return items
  return items.filter(item => {
    const label = normalizeText(item.label)
    return words.every(word => label.includes(word))
  })
}

export interface FolderSearchNode {
  folder?: { id: string; name: string; path: string; type: string } | null
  children?: FolderSearchNode[]
}
export interface FolderChoice extends SearchableFolder {
  accountId: string
  accountName: string
  accountEmail: string
  name: string
  path: string
  type: string
}

/** Flatten cached trees so search reaches folders inside collapsed branches. */
export function folderChoices(accountId: string, accountName: string, accountEmail: string, trees: FolderSearchNode[]): FolderChoice[] {
  const result: FolderChoice[] = []
  const visit = (nodes: FolderSearchNode[]) => {
    for (const node of nodes) {
      const item = node.folder
      if (item) result.push({ id: item.id, accountId, accountName, accountEmail, name: item.name, path: item.path, type: item.type, label: `${accountName} ${accountEmail} ${item.path} ${item.name}` })
      if (node.children?.length) visit(node.children)
    }
  }
  visit(trees)
  return result
}
