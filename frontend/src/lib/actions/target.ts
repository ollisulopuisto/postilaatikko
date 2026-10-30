import type { ListTarget, ListTargetSource } from './types'

/**
 * The one statement of the message-action target hierarchy from
 * docs/KEYBOARD_SHORTCUTS.md: checked messages if any are checked, otherwise
 * the keyboard-focused row, otherwise nothing. The key handlers, the command
 * panel and any contextual surface call this instead of repeating it.
 */
export function resolveListTarget(source: ListTargetSource): ListTarget {
  if (source.hasChecked()) {
    const refs = source.checkedRefs?.() ?? []
    return {
      kind: 'checked',
      messageIds: source.checkedIds(),
      actionTarget: refs.length ? { kind: 'matched_messages', messages: refs } : null,
      star: source.checkedHasUnstarred(),
      read: source.checkedHasUnread(),
    }
  }
  const focused = source.focusedIds()
  if (focused.length > 0) {
    const refs = source.focusedRefs?.() ?? []
    return {
      kind: 'focused',
      messageIds: focused,
      actionTarget: refs.length ? { kind: 'matched_messages', messages: refs } : null,
      star: !source.selectedStarred(),
      read: source.selectedUnread(),
    }
  }
  return { kind: 'none', messageIds: [], actionTarget: null, star: true, read: false }
}
