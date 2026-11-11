import { findFocusedRow, type FocusableRow } from './focusedRow'

/**
 * Folder-navigation selection policy for the message list, extracted so the
 * navigation behavior is testable without mounting the component.
 *
 * The reading pane stays empty until the user picks a conversation: a folder
 * with no remembered state yields no selection, and a same-folder refresh
 * keeps the selection only while its account-qualified row is present.
 */

/**
 * Selection to apply after switching folders: the remembered row when it is
 * still in the list, otherwise none. Never falls back to the first row.
 */
export function selectionAfterFolderSwitch<T extends FocusableRow>(
  rows: T[],
  remembered: { selectedThreadId: string | null; focusedRowIdentity: string | null } | undefined,
  accountId = '',
  folderId = '',
): T | undefined {
  if (!remembered?.selectedThreadId) return undefined
  if (!rows.some((row) => row.threadId === remembered.selectedThreadId)) return undefined
  return findFocusedRow(rows, remembered.focusedRowIdentity, remembered.selectedThreadId, accountId, folderId)
}

/**
 * A same-folder refresh keeps the selection only while its account-qualified
 * row is present. A vanished conversation (snoozed or moved from another
 * client) must not linger as a phantom selection that Space can turn into a
 * bulk action on nothing.
 */
export function selectionSurvivesRefresh<T extends FocusableRow>(
  rows: T[],
  focusedRowIdentity: string | null,
  selectedThreadId: string | null,
  accountId = '',
  folderId = '',
): boolean {
  if (!selectedThreadId) return false
  return findFocusedRow(rows, focusedRowIdentity, selectedThreadId, accountId, folderId) !== undefined
}
