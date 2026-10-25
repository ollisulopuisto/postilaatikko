// Calendar shortcut descriptions for the in-app shortcuts overlay and
// tooltip hints. Discovered by $lib/keyboard/registry.ts via import.meta.glob;
// predicates come from ./shortcuts.ts, so the registry test can check that
// the displayed keys still match the bindings registered in CalendarPane.

import type { ExtensionShortcutRegistry } from '$lib/keyboard/registry'
import { KEY } from './shortcuts'

const group = 'calendar'

const registry: ExtensionShortcutRegistry = {
  extensionId: 'calendar',
  groups: [{ id: group, label: 'calendar.shortcuts.group' }],
  predicates: KEY,
  shortcuts: [
    { id: 'calendar.today', label: 'calendar.shortcuts.today', group, combos: ['t'], predicates: [KEY.CALENDAR_TODAY] },
    { id: 'calendar.prev', label: 'calendar.shortcuts.prev', group, combos: ['ArrowLeft'], predicates: [KEY.CALENDAR_PREV] },
    { id: 'calendar.next', label: 'calendar.shortcuts.next', group, combos: ['ArrowRight'], predicates: [KEY.CALENDAR_NEXT] },
    { id: 'calendar.viewMonth', label: 'calendar.shortcuts.viewMonth', group, combos: ['Alt+m'], predicates: [KEY.CALENDAR_VIEW_MONTH] },
    { id: 'calendar.viewWeek', label: 'calendar.shortcuts.viewWeek', group, combos: ['Alt+w'], predicates: [KEY.CALENDAR_VIEW_WEEK] },
    { id: 'calendar.viewDay', label: 'calendar.shortcuts.viewDay', group, combos: ['Alt+d'], predicates: [KEY.CALENDAR_VIEW_DAY] },
    { id: 'calendar.viewAgenda', label: 'calendar.shortcuts.viewAgenda', group, combos: ['Alt+a'], predicates: [KEY.CALENDAR_VIEW_AGENDA] },
    { id: 'calendar.newEvent', label: 'calendar.shortcuts.newEvent', group, combos: ['Mod+n'], predicates: [KEY.CALENDAR_NEW_EVENT] },
    { id: 'calendar.sync', label: 'calendar.shortcuts.sync', group, combos: ['Mod+r', 'Mod+Shift+a'], predicates: [KEY.CALENDAR_SYNC, KEY.CALENDAR_SYNC_ALL] },
    { id: 'calendar.focus', label: 'calendar.shortcuts.focus', group, combos: ['f'], predicates: [KEY.CALENDAR_FOCUS_TOGGLE] },
  ],
}

export default registry
