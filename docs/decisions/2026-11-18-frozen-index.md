# Frozen message index

The loaded view freezes account-qualified thread order and inbox card membership.
Reading, selecting another row, keyboard navigation, hover/prefetch and backend
sync/arrival/read events do not release it. Read indicators still update. External
removals remain represented until reconciliation; actions still resolve canonical
backend targets through the existing guarded mutation paths. Opening an excluded
retained row first verifies its message still exists; unavailable rows show a
re-sort notice rather than opening a broken reader.

The pending-change pill counts distinct changed/new/removed identities, not event
notifications. Reading can increase this count because reconciling would move
the row between Read/Priority cards; it never changes its frozen position.
Clicking it reconciles and refreshes the loaded window, without
selecting or marking another message read. It is a normal focusable button above
the rows, not an overlay covering them. Dark mode and forced-colour screenshots
are generated from the actual App with deterministic Wails fixtures.

## Deliberate actions that release order

The examples from the owner were not an exhaustive whitelist. This is the audit
list for review and veto:

| Action | Integration point |
| --- | --- |
| Archive, archive-and-read, Done/remove from Inbox, Trash, permanent delete, empty Trash, spam/not-spam, move/copy/move to Inbox, explicit mark read/unread, star/unstar, message menu mutations | Existing row/reader/bulk completion callbacks into `doHandleActionComplete`, before the fresh query, after capturing the auto-next origin |
| Later add/remove from toolbar, per-message or list row; Later bulk/menu and undo | Completion callbacks, bulk queue completion; undo-operation event and Later undo callback |
| Snooze from shared viewer/keyboard dialog, or row menu | Shared dialog `onDone` and existing context menu completion |
| Undo from the list toast or completed mutation event | `handleUndo`, `handleUndoOperationCompleted`; reader/bulk existing completion callbacks |
| Compose new, edit draft, mailto/address compose | App compose entry points, including detached compose |
| Reply/reply-all/forward | App entry plus reader `onReplyStart` for inline reply, which does not call the full composer entry |
| Folder/account/view switch | Existing folder-change effect and list remount |
| All/Unread/Later filters, sort direction | `setFilter`, `toggleSortOrder` |
| Inbox Categories/Priority/Chronological and card settings | Display/settings handlers |
| Expand/collapse inbox card, Show all card rows | Group presentation handlers |
| Search query/reset/source/refresh and all-mail search | Search entry/reset handlers; search result arrays remain query-owned and passive folder sync cannot replace them |
| Refresh/Cmd-R, forced refresh, pending-change pill | `syncFolder`, `forceSyncFolder`, `refreshActiveSearch`, `showNewestMail` |
| Load more | Appends requested new rows while retaining all existing frozen positions; not a whole-view re-sort |

Checkbox/range selection, focus movement, opening/closing the context menu and
opening a message are not release actions. A canceled mutation does not release
through a success callback. Compose/reply release when started, not when sent.
No backend, mailbox schema, message body fetching or remote sorting is changed.

## Verification

Pure module tests cover read updates, stable grouping basis, identity collisions
across accounts, arrivals, removals, release and explicit pagination. Tests were
observed failing against a pass-through row implementation before restoring the
fixed module. Full-App browser coverage opens one message, marks it read, selects
another, injects a new/reordered backend window and verifies unchanged DOM row
identities in grouped and plain folders. It then clicks the pill and starts an
inline reply after another arrival, verifying both deliberate releases.

Review regressions cover compose followed by pagination/passive arrival, external
removal at the first-page boundary, last live read state after unread-filter
exclusion, detached viewer false/true completion forwarding, detached compose and
full/inline reply, and Later mutation/undo completion. New behavioral tests were
run against the previous PR head and observed failing before restoring the fixes.
Explicit replacement permission is local to each query, never a lingering view
flag. Standard folders re-fetch a growing header prefix when loading more, so
external removals cannot skip a boundary row; virtual folders retain their own
provider cursor. Retained display rows do not count as backend progress.
