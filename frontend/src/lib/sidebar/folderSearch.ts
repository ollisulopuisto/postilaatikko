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
