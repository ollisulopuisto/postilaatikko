// Contacts shortcut descriptions for the in-app shortcuts overlay and
// tooltip hints. Discovered by $lib/keyboard/registry.ts via import.meta.glob;
// predicates come from ./shortcuts.ts, so the registry test can check that
// the displayed keys still match the bindings registered in ContactsPane.

import type { ExtensionShortcutRegistry } from '$lib/keyboard/registry'
import { KEY } from './shortcuts'

const group = 'contacts'

const registry: ExtensionShortcutRegistry = {
  extensionId: 'contacts',
  groups: [{ id: group, label: 'contacts.shortcuts.group' }],
  predicates: KEY,
  shortcuts: [
    { id: 'contacts.edit', label: 'contacts.shortcuts.edit', group, combos: ['e'], predicates: [KEY.CONTACT_EDIT] },
    { id: 'contacts.new', label: 'contacts.shortcuts.new', group, combos: ['Mod+n'], predicates: [KEY.CONTACT_NEW] },
    { id: 'contacts.syncAll', label: 'contacts.shortcuts.syncAll', group, combos: ['Mod+Shift+a'], predicates: [KEY.CONTACT_SYNC_ALL] },
    { id: 'contacts.syncFocused', label: 'contacts.shortcuts.syncFocused', group, combos: ['Mod+Shift+s'], predicates: [KEY.CONTACT_SYNC_FOCUSED] },
  ],
}

export default registry
