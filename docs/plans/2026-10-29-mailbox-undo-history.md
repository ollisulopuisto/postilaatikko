# Mailbox undo history - state and open decision (2026-10-29)

Working note for the morning review. It records what exists and the one open
product decision. It does not settle that decision.

## What exists now (branch feat/mailbox-undo-history, unpublished)

- Durable mailbox undo: every undoable mailbox action (archive/move, flag
  changes; send and permanent delete are excluded) is written to
  `mailbox_undo_commands` (migration 59) when it happens.
- Undo survives app restart. Retention is 24 hours from each action; there is
  no count cap, so the full 24h window is preserved and its actual growth can
  be measured (attach/sweep logs report stored row counts).
- Undo order is LIFO: Ctrl+Z reverses the most recent action first.
- Failed undos stay undoable (the operation is restored, matching the
  existing in-memory behavior).

## Open decision for the user: Kuulas-style branching/redo

The user's original guidance: "take heed from the way Kuulas uses undo and
allows forking" and "it's got forking history and clever undo states", plus
"if you're not sure what 'infinite undo' means in every case, ask me".

Kuulas keeps a tree: after undoing and doing something new, the undone branch
still exists and you can return to it. What that should mean for server-side
mail actions is genuinely ambiguous:

- **Option A - undo-only history (current build).** LIFO within 24h. Simple
  and predictable: each undo reverses one real server action. No redo.
- **Option B - redo of undone actions.** After an undo, a redo re-executes
  the original action (move again, re-apply flags). Needs forward payloads in
  addition to inverse ones; conflicts are possible if the mailbox changed
  meanwhile (message moved elsewhere on another client) - those must surface
  as visible failures, same as undo failures today.
- **Option C - full branching like Kuulas.** Undoing and then acting keeps
  both branches browsable. For mail this mostly matters as a history/audit
  view; the useful daily part is usually B.

Storage impact of B/C is small (same rows plus a state marker); the real cost
is UI (how redo/branch is exposed) and conflict semantics.

## Also found while building (separate branch fix/draft-blob-migration58)

PR #45's migration 58 never landed on main, so on current main the composer
attachment-undo tables don't exist: send/discard cleanup errors (swallowed by
a frontend catch, leaking history rows) and attaching a file breaks history
snapshots. Verified against main's real migration chain; regression test
committed on the fix branch.
