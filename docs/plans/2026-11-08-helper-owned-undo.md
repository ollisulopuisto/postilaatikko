# HOSTED-SERVER-056: Helper-Owned Undo

## Goal

Move reversible mail action state and Undo execution into the helper so remote and localhost clients can use the same durable operation. A successful action must not be advertised as undoable unless its privacy-minimal snapshot was committed with the receipt.

## Scope

- Extend the durable action receipt schema with action kind, safe summary timestamps, undo state, expiry, and the minimum prior-state snapshot needed for supported inverse operations.
- Capture previous folder and flags from helper-owned message rows before provider calls. Include account-qualified message/folder IDs, never message content or headers.
- Persist the snapshot atomically with the terminal successful action receipt. Ambiguous or partially failed actions remain non-undoable until reconciliation.
- Implement helper-side inverse operations for archive, mark-read/unread, star/unstar, trash, and move, restoring each message's exact prior folder/flag state.
- Add authenticated, bounded Undo history, Undo latest, and Undo by operation endpoints. Reuse `mail.write` for Undo and expose no message IDs in history.
- Require a caller idempotency key for both Undo endpoints; a retry of Undo latest must replay the same operation instead of undoing the next older operation.
- Atomically claim an undo operation and persist `undoing`, `undone`, or `outcome_unknown` state. Retries must never double-apply provider effects.

## Safety boundaries

- Service DB/message/folder/provider state is authoritative. Do not use desktop stores, Undo stack, or provider pools.
- Keep destination-copy-before-source-delete when reversing cross-account moves; do not delete a source until the destination copy is confirmed.
- Expire normal Undo eligibility after 24 hours. Retain uncertain records for explicit reconciliation.
- Because a process can stop between external provider success and local checkpoint, startup must fail closed and must not blindly retry an undo left in `undoing`.
- Unsupported or incomplete snapshots return an explicit non-undoable result.

## Acceptance

1. Action receipt and undo snapshot commit together; no content, headers, credentials, or attachment data enter the snapshot.
2. History is newest-first, bounded, and omits message identifiers and mail content.
3. Undo latest selects and claims the newest eligible operation atomically; two concurrent claims cannot execute it twice.
4. Undo by operation is idempotent and only succeeds for an owned, unexpired, confirmed operation.
5. Provider-confirmed reversal restores exact prior flags/folders. Ambiguous provider results remain `outcome_unknown` and are never replayed automatically.
6. Existing helper action behavior still builds; focused tests and `git diff --check` pass when permitted by session instructions.

## Implementation sequence

1. Inspect provider move/flag behavior and helper message fields for reliable before-state snapshots.
2. Add a forward-only migration and typed snapshot/history/undo-store contracts.
3. Capture snapshots in `mailboxActionOperations`; persist them in the durable receipt completion transaction.
4. Add undo operation methods and authenticated routes with atomic state claims.
5. Review API privacy, restore ordering, restart recovery, and update `TASKS.md` with exact checks and limitations.
