# Folder-first navigation and sidebar groups

Status: product direction accepted by the user on 2026-10-26; implementation is P-038.

## Goal

Give the left rail and folder sidebar to the mail folders the user opens most. Contacts and Calendar should not take rail space by default. Keep them available through a preference.

## Navigation model

### Left rail: quick access

- Use the existing pinned-folder list as the user's ordered quick-access list. Do not infer or upload usage history.
- Show pinned folders in the left rail, one click to open the folder. Reordering pins changes their rail position.
- Bind the first ten positions to `Cmd+1` through `Cmd+0` on macOS and `Ctrl+1` through `Ctrl+0` on other platforms. `1` is first and `0` is tenth. Ignore empty positions and do not fire these while editing text or composing.
- Hide Contacts and Calendar rail entries by default. Add a Preferences option to show or hide those entries; changing it must not disable either extension or delete its data.
- Keep extension destinations available from their preference-controlled rail entries, and retain a discoverable recovery path in Preferences.

### Folder sidebar: group cards

- Represent sidebar organization as ordered group cards. Each card can be dragged to reorder, expanded/collapsed, or hidden. Hidden cards and their contents remain recoverable in sidebar settings.
- Let the user move supported entries between cards. Moving an entry changes presentation only; it must not move mail on the server or change its account folder.
- Keep system destinations, virtual folders, and pinned/starred real folders as distinct entry types. A virtual-folder card and a pinned/starred-folders card are independent cards.
- Suggested initial cards:
  - **Core:** Starred, Drafts, Sent, Trash.
  - **Later:** Archived, Blocked, Snoozed.
  - **Virtual folders:** all saved virtual folders.
  - **Starred folders:** the user's pinned real folders.
- Preserve the existing all-folder browser and account folder tree, so ungrouped, unpinned, or hidden folders remain reachable.
- Keep account-tree expansion state separate from card collapse and card visibility.

## Persistence and migration

- Persist card order, visibility, collapsed state, card membership, and the optional Contacts/Calendar visibility preference in UI state.
- Migrate existing sidebar order/hidden preferences without losing them. Start with the suggested cards for newly created layouts; preserve existing pins and virtual folders.
- Normalize malformed or stale saved layouts by restoring known defaults and keeping unknown entries recoverable where possible.

## Interaction and accessibility

- Drag targets must also be operable from the keyboard. Provide move earlier/later and move to group actions in a menu or settings surface.
- Give card headers clear expand/collapse and visibility controls with localized accessible names.
- Announce keyboard shortcut assignments and show folder names in rail tooltips.
- Keep `Cmd/Ctrl+1…0` consistent with platform conventions, and ensure typing fields and the composer retain their keys.

## Implementation slices

1. Extend persisted UI-state types and normalization for group cards, migration from the existing P-008 order/visibility model, and rail visibility preferences.
2. Render ordered pinned folders in the rail and add platform-aware `Cmd/Ctrl+1…0` navigation, with shortcut documentation and conflict checks.
3. Render group cards in the folder sidebar, with defaults, reorder, collapse, hide/restore, and move-entry actions.
4. Verify migrations, keyboard-only operation, responsive layouts, hidden-item recovery, and that moving entries never mutates server-side folders.

## Out of scope

- Automatically tracking folder usage or sending usage telemetry.
- Changing mailbox contents, labels, or account folder hierarchy when rearranging cards.
- Removing Contacts or Calendar extensions; this only changes their default rail visibility.
