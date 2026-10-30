# Mailbox Undo History UI

## Product behavior

- `Cmd-Z` on macOS (`Ctrl-Z` elsewhere) remains the immediate undo for the
  latest undoable mailbox operation. The Edit menu's Undo item uses the same
  operation and reflects its current description when the platform allows.
- Add **Edit > Undo History…**. It opens a compact, keyboard-accessible panel
  in the active mail window; it is not a separate permanent window.
- Show recent durable mailbox operations in reverse chronological order, with
  a concise action description, relative time, and whether the operation is
  still undoable. An explicit per-row Undo targets only that operation.
- Do not imply redo or branching. The current backend supports undo-only
  history. Expired, already-undone, and currently executing operations are
  visible as unavailable only if the durable API can report them accurately;
  otherwise omit them.
- Keep the panel useful when empty, during loading, and after a failed undo.
  Refresh after mailbox actions and `undo:completed` events. Do not display
  message bodies, recipients, or other unnecessary private content.

## Acceptance

1. Existing `Cmd-Z`/`Ctrl-Z` still performs only the latest undoable action.
2. Edit > Undo History opens the compact panel in the active mail window.
3. Each available row invokes `UndoOperation(operationId)` and refreshes the
   list; one row cannot be undone twice concurrently.
4. Loading, empty, failure, and unavailable states are clear and localized.
5. Keyboard navigation, Escape dismissal, and focus restoration work.
6. The history list is bounded and fetched on panel open/refresh, not polled.

## Implementation notes

Reuse the durable `mailbox_undo_commands` store and operation-scoped undo API.
Do not add redo, branching, a new retention policy, or a message-content view.
If current storage cannot provide an accurate ordered list of operation-level
summaries and availability, add the smallest read-only API needed without
changing undo semantics.

## Implementation checkpoint (2026-10-30)

- History reads the existing durable undo stack, groups multi-command actions,
  returns newest first, and caps the result at 100 rows. The UI requests at
  most 50 and includes no message IDs, addresses, or body content.
- Cmd-Z/Ctrl-Z and native Edit → Undo share global LIFO. Toast and history-row
  Undo remain operation-scoped. The Undo History dialog is mounted separately
  from the shortcuts overlay and opens from the native Edit menu.
- Verification: frontend Svelte check reports zero errors and two existing
  title-bar warnings; shortcut checks pass. The frontend check emitted a Vite
  WebSocket `EPERM` warning in the sandbox. Go and production builds passed
  earlier in this task branch. Live macOS menu behavior is still unverified.
