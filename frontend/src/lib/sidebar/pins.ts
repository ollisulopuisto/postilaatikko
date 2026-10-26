// Pinned folders for the sidebar (P-007).
//
// A pin is identified by account and folder together. Folder ids are only
// unique within an account, so two accounts can each have an "Inbox" pinned.
// These helpers return new arrays and never mutate their input.

export interface PinnedFolder {
  accountId: string
  folderId: string
  path: string
  name: string
}

function sameFolder(pin: PinnedFolder, accountId: string, folderId: string): boolean {
  return pin.accountId === accountId && pin.folderId === folderId
}

// Returns whether the account's folder is pinned.
export function isPinned(pins: PinnedFolder[], accountId: string, folderId: string): boolean {
  return pins.some(pin => sameFolder(pin, accountId, folderId))
}

// Adds a pin at the end of the list. Pinning an already pinned folder returns
// the list unchanged.
export function pinFolder(pins: PinnedFolder[], pin: PinnedFolder): PinnedFolder[] {
  if (isPinned(pins, pin.accountId, pin.folderId)) return pins
  return [...pins, { accountId: pin.accountId, folderId: pin.folderId, path: pin.path, name: pin.name }]
}

// Removes the account's folder pin. Removing an absent pin returns the list
// unchanged.
export function unpinFolder(pins: PinnedFolder[], accountId: string, folderId: string): PinnedFolder[] {
  if (!isPinned(pins, accountId, folderId)) return pins
  return pins.filter(pin => !sameFolder(pin, accountId, folderId))
}

// Pins the folder when it is unpinned and unpins it when it is pinned.
export function togglePin(pins: PinnedFolder[], pin: PinnedFolder): PinnedFolder[] {
  if (isPinned(pins, pin.accountId, pin.folderId)) return unpinFolder(pins, pin.accountId, pin.folderId)
  return pinFolder(pins, pin)
}

function isPinnedFolder(value: unknown): value is PinnedFolder {
  if (typeof value !== 'object' || value === null) return false
  const candidate = value as Record<string, unknown>
  return ['accountId', 'folderId', 'path', 'name'].every(key => typeof candidate[key] === 'string')
    && (candidate.accountId as string) !== ''
    && (candidate.folderId as string) !== ''
}

// Cleans pins read from the persisted UI state. Entries that are not objects
// with string fields are dropped, and a repeated account and folder pair keeps
// only its first occurrence. Anything that is not an array yields no pins.
export function normalizePins(raw: unknown): PinnedFolder[] {
  if (!Array.isArray(raw)) return []
  const result: PinnedFolder[] = []
  for (const entry of raw) {
    if (!isPinnedFolder(entry)) continue
    if (isPinned(result, entry.accountId, entry.folderId)) continue
    result.push({ accountId: entry.accountId, folderId: entry.folderId, path: entry.path, name: entry.name })
  }
  return result
}
