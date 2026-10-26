import { getSidebarGroups, setSidebarGroups } from './uiState.svelte'
import {
  moveSidebarEntry,
  moveSidebarGroup,
  normalizeSidebarGroups,
  reorderSidebarGroups,
  setSidebarGroupHidden,
  toggleSidebarGroupCollapsed,
  type SidebarGroupEntry,
  type SidebarGroups,
} from '../sidebar/groups'

let groups = $state<SidebarGroups>(getSidebarGroups())

function commit(next: SidebarGroups): void {
  groups = normalizeSidebarGroups(next)
  setSidebarGroups(groups)
}

export const sidebarGroups = {
  get list(): SidebarGroups {
    return groups
  },
  move(groupId: string, delta: -1 | 1): void {
    commit(moveSidebarGroup(groups, groupId, delta))
  },
  reorder(fromId: string, toId: string): void {
    commit(reorderSidebarGroups(groups, fromId, toId))
  },
  setHidden(groupId: string, hidden: boolean): void {
    commit(setSidebarGroupHidden(groups, groupId, hidden))
  },
  toggleCollapsed(groupId: string): void {
    commit(toggleSidebarGroupCollapsed(groups, groupId))
  },
  moveEntry(entry: SidebarGroupEntry, toGroupId: string): void {
    commit(moveSidebarEntry(groups, entry, toGroupId))
  },
  reset(): void {
    commit(normalizeSidebarGroups(undefined))
  },
  sync(): void {
    groups = normalizeSidebarGroups(getSidebarGroups())
  },
}
