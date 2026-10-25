import type { ListTarget, ListTargetSource } from './types'

/**
 * The one statement of the message-action target hierarchy from
 * docs/KEYBOARD_SHORTCUTS.md: checked messages if any are checked, otherwise
 * the keyboard-focused row, otherwise nothing. The key handlers, the command
 * panel and any contextual surface call this instead of repeating it.
 */
export function resolveListTarget(source: ListTargetSource): ListTarget {
  if (source.hasChecked()) {
    return { kind: 'checked', messageIds: source.checkedIds(), star: source.checkedHasUnstarred() }
  }
  const focused = source.focusedIds()
  if (focused.length > 0) {
    return { kind: 'focused', messageIds: focused, star: !source.selectedStarred() }
  }
  return { kind: 'none', messageIds: [], star: true }
}
