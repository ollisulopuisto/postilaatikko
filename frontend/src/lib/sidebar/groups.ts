export const SIDEBAR_GROUP_ENTRIES = [
  'special:starred',
  'special:drafts',
  'special:sent',
  'special:trash',
  'special:archive',
  'special:spam',
  'special:snoozed',
  'virtual-folders',
  'starred-folders',
] as const

export type SidebarGroupEntry = (typeof SIDEBAR_GROUP_ENTRIES)[number]

export interface SidebarGroup {
  id: string
  titleKey: string
  entries: SidebarGroupEntry[]
  collapsed: boolean
  hidden: boolean
}

export type SidebarGroups = SidebarGroup[]

export const DEFAULT_SIDEBAR_GROUPS: SidebarGroups = [
  {
    id: 'core',
    titleKey: 'sidebar.groupCore',
    entries: ['special:starred', 'special:drafts', 'special:sent', 'special:trash'],
    collapsed: false,
    hidden: false,
  },
  {
    id: 'later',
    titleKey: 'sidebar.groupLater',
    entries: ['special:archive', 'special:spam', 'special:snoozed'],
    collapsed: true,
    hidden: false,
  },
  {
    id: 'virtual',
    titleKey: 'sidebar.groupVirtualFolders',
    entries: ['virtual-folders'],
    collapsed: false,
    hidden: false,
  },
  {
    id: 'starred-folders',
    titleKey: 'sidebar.groupStarredFolders',
    entries: ['starred-folders'],
    collapsed: false,
    hidden: false,
  },
]

const isEntry = (value: unknown): value is SidebarGroupEntry =>
  typeof value === 'string' && (SIDEBAR_GROUP_ENTRIES as readonly string[]).includes(value)

export function normalizeSidebarGroups(raw: unknown): SidebarGroups {
  const source = Array.isArray(raw) ? raw : []
  const known = new Map(DEFAULT_SIDEBAR_GROUPS.map(group => [group.id, group]))
  const groups: SidebarGroups = []
  const claimed = new Set<SidebarGroupEntry>()

  for (const candidate of source) {
    if (!candidate || typeof candidate !== 'object') continue
    const value = candidate as Partial<SidebarGroup> & { id?: unknown; entries?: unknown }
    const defaultGroup = known.get(String(value.id ?? ''))
    if (!defaultGroup || groups.some(group => group.id === defaultGroup.id)) continue
    const entries = Array.isArray(value.entries)
      ? value.entries.filter(isEntry).filter(entry => !claimed.has(entry))
      : []
    for (const entry of entries) claimed.add(entry)
    groups.push({
      ...defaultGroup,
      entries,
      collapsed: value.collapsed === true,
      hidden: value.hidden === true,
    })
  }

  for (const group of DEFAULT_SIDEBAR_GROUPS) {
    let existing = groups.find(item => item.id === group.id)
    if (!existing) {
      existing = { ...group, entries: [] }
      groups.push(existing)
    }
    for (const entry of group.entries) {
      if (claimed.has(entry)) continue
      existing.entries.push(entry)
      claimed.add(entry)
    }
  }

  return groups
}

// Map the P-008 sidebar block layout into the first card layout. The unified
// inbox remains its own navigation section; these mappings preserve the order
// and visibility of the old pinned, virtual, primary and secondary sections.
export function migrateSidebarGroups(raw: unknown, legacyOrder: unknown, legacyHidden: unknown): SidebarGroups {
  if (Array.isArray(raw) && raw.length > 0) return normalizeSidebarGroups(raw)
  if (!Array.isArray(legacyOrder) && !Array.isArray(legacyHidden)) return normalizeSidebarGroups(undefined)

  const legacyToGroup: Record<string, string> = {
    primary: 'core',
    secondary: 'later',
    virtual: 'virtual',
    pinned: 'starred-folders',
  }
  const defaults = normalizeSidebarGroups(undefined)
  const groupOrder: string[] = []
  for (const block of Array.isArray(legacyOrder) ? legacyOrder : []) {
    const groupId = legacyToGroup[String(block)]
    if (groupId && !groupOrder.includes(groupId)) groupOrder.push(groupId)
  }
  for (const group of defaults) if (!groupOrder.includes(group.id)) groupOrder.push(group.id)

  const legacyHiddenSet = new Set(Array.isArray(legacyHidden) ? legacyHidden.map(String) : [])
  const hiddenGroups = new Set(
    Object.entries(legacyToGroup)
      .filter(([block]) => legacyHiddenSet.has(block))
      .map(([, groupId]) => groupId),
  )
  const byId = new Map(defaults.map(group => [group.id, group]))
  return groupOrder.map(id => {
    const group = byId.get(id)!
    return { ...group, entries: [...group.entries], hidden: hiddenGroups.has(id) }
  })
}

export function moveSidebarGroup(groups: SidebarGroups, groupId: string, delta: -1 | 1): SidebarGroups {
  const from = groups.findIndex(group => group.id === groupId)
  const to = from + delta
  if (from < 0 || to < 0 || to >= groups.length) return groups
  const next = [...groups]
  ;[next[from], next[to]] = [next[to], next[from]]
  return next
}

export function setSidebarGroupHidden(groups: SidebarGroups, groupId: string, hidden: boolean): SidebarGroups {
  return groups.map(group => group.id === groupId ? { ...group, hidden } : group)
}

export function toggleSidebarGroupCollapsed(groups: SidebarGroups, groupId: string): SidebarGroups {
  return groups.map(group => group.id === groupId ? { ...group, collapsed: !group.collapsed } : group)
}

export function moveSidebarEntry(groups: SidebarGroups, entry: SidebarGroupEntry, toGroupId: string): SidebarGroups {
  if (!isEntry(entry) || !groups.some(group => group.id === toGroupId)) return groups
  return groups.map(group => ({
    ...group,
    entries: group.id === toGroupId
      ? [...group.entries.filter(item => item !== entry), entry]
      : group.entries.filter(item => item !== entry),
  }))
}

export function reorderSidebarGroups(groups: SidebarGroups, fromId: string, toId: string): SidebarGroups {
  const from = groups.findIndex(group => group.id === fromId)
  const to = groups.findIndex(group => group.id === toId)
  if (from < 0 || to < 0 || from === to) return groups
  const next = [...groups]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}
