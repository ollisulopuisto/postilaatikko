# Server-Owned Mail Actions and Undo Design

**Goal:** Make the mailbox helper the durable authority for remote mail mutations, safe retries, operation history, and Undo while keeping the desktop a responsive client.

**Architecture:** The helper already resolves account-qualified `operations.Target` values and runs provider mutations through `internal/alwayson/mailboxActionOperations`; the missing pieces are durable action receipts, reversible operation state, and authenticated undo endpoints. Add a service-owned journal around provider groups so accepted, partial, and uncertain outcomes survive process restart. Store only the account/message/folder/provider identifiers and previous flag/folder state needed for reversal; keep message content and headers out of undo history. Remote and local-helper desktops call the same helper contract, while existing desktop-local mutation behavior remains available only when the localhost helper is unavailable.

**Tech Stack:** Go, `internal/automation/v1`, `internal/alwayson`, SQLite migrations, `internal/automation/client`, device-owned desktop state and Wails events.

---

## Current-state evidence

- `internal/automation/v1/actions.go` resolves explicit account-qualified targets and performs archive, flags, Trash, and move. Idempotency receipts are held in `ActionsHandler` memory, so a helper restart forgets them.
- `internal/alwayson/actions_api.go` loads helper-owned message rows and performs provider work. Cross-account flag groups can partially succeed; the action API currently reports one success/error boundary and does not persist each completed group for recovery.
- `app/actions.go` routes connected local-helper mutations through the helper, but constructs message objects from the desktop mailbox database and writes Undo commands into the desktop's `undoStack`.
- `app/undo.go` relies on desktop stores, IMAP pool and `undo.UndoContext`. Remote-only startup intentionally creates none of those.
- The helper database already has `mailbox_undo_commands` (migration 59), but its payloads currently encode desktop-local undo command types and are rehydrated by the desktop. That table is not yet a service-owned action journal.

## Required invariants

1. Resolve and authorize a target against the helper's current account/message rows before any provider call; all actions are account-qualified.
2. Persist the operation ID, idempotency key hash, canonical request hash, current state, and provider-group progress before a response could be lost. Raw bearer tokens, recipient/content data, and complete message headers never enter receipts/history.
3. A retry with the same key and same request returns the exact durable outcome. Reusing the key with a different target/action is a conflict. An uncertain provider outcome is never replayed as a new mutation automatically.
4. Record enough prior state to reverse the action only after confirming the provider mutation. Persist successful provider groups independently; partial outcomes expose which groups succeeded and which need reconciliation.
5. Undo claims an operation atomically. A failed or uncertain undo remains visible and cannot be concurrently applied twice. The operation becomes completed only after provider and local service state agree.
6. Mutation and undo responses include affected account/folder/message references sufficient for bounded desktop refresh events, but no message body. Change notification loss must be recoverable from the durable cursor/operation state.
7. The helper database is authoritative in both localhost and remote deployments. Remote authorization/capability errors fail closed; they never activate local database fallback.

## Proposed v1 contract

- `POST /v1/actions` remains the capability-checked mutation route. Require `Idempotency-Key`, resolve the target server-side, and return a durable operation receipt plus safe affected references and partial-result details.
- `GET /v1/undo/history?limit=` returns a bounded newest-first list containing operation ID, action label, timestamps, expiry, message count, state and whether Undo is currently available. It omits message IDs and mail content.
- `POST /v1/undo/operations/{operationID}` undoes one operation idempotently and returns its terminal or uncertain receipt.
- `POST /v1/undo/latest` atomically claims the newest undoable action for Cmd-Z / Edit → Undo.
- Reuse `mail.write` for mutation/undo authorization. History may use `mail.read`; do not add an always-on remote capability unless server policy proves the existing boundary is inadequate.
- Client methods retain the same request key after a network timeout, query receipt state before retrying, and surface `outcome_unknown` without issuing a fresh action key.

## Durable storage and recovery

- Add service-owned `mail_action_operations` and `mail_action_groups` records (or an equivalent normalized schema). The parent row stores request/idempotency hashes, action, state, safe timestamps, and public summary. Group rows store account/provider group, source/destination folder IDs, affected helper message IDs, pre-mutation flags/folder IDs, and provider completion/uncertainty state.
- Use a digest/HMAC of idempotency keys for database indexing; never persist the raw key. Keep the canonical request fingerprint to reject conflicting retries.
- Persist the operation row before provider effects, then checkpoint each provider group. Do not wrap external provider operations in a misleading SQLite transaction. Recover `in_progress` after restart as uncertain and require provider reconciliation; never replay an uncertain move/send blindly.
- Preserve destination-copy-before-source-delete for cross-account moves. A partial cross-account operation remains undoable only for confirmed groups, with an explicit partial status for the rest.
- Use a bounded retention policy consistent with the existing 24-hour desktop Undo window. Sweep only expired terminal records; retain uncertain records until resolved or explicitly acknowledged.

## Dependency-ordered implementation slices

1. **HOSTED-SERVER-055 — persistent mutation receipts:** add the operation/group journal, action handler integration, restart/idempotency/partial-group tests, and safe receipt lookup.
2. **HOSTED-SERVER-056 — helper-owned Undo execution:** snapshot previous state for each supported action, expose atomic `undo latest` and `undo by operation`, and verify provider-confirmed state transitions and recovery behavior.
3. **HOSTED-SERVER-057 — authenticated client and app adapter:** add client/history methods and route Wails action/Undo calls through the configured helper without reading local message rows in remote mode.
4. **HOSTED-SERVER-058 — frontend reconciliation/events:** update only affected account/folder/thread state from action responses and helper change cursors; preserve current selection/undo UX and show partial/unknown outcomes accurately.
5. Retire duplicated desktop Undo/mutation work only after local-helper and remote integration tests prove equivalent behavior; retain local fallback strictly for the localhost-helper-unavailable mode.

## Acceptance criteria

- A helper restart after a successful action returns the original receipt for the same idempotency key and does not repeat provider effects.
- Same key with a changed action/target conflicts; ambiguous outcomes remain non-replayable until reconciled.
- Cross-account partial flag/move outcomes persist per-account completion and reconcile the desktop from provider-confirmed state.
- Undo of each supported action restores its exact prior provider state, remains durable across helper restart, and cannot double-apply under concurrent requests.
- Wrong-account, stale, duplicate, empty, or unknown targets fail before mutation. Responses/history omit mail content and raw message IDs.
- The configured remote desktop performs mutation and Undo with no mailbox DB, local message/account/credential store, or provider pool; helper auth/capability/network errors fail closed.
- Local-helper mode remains compatible, and unsupported operations return explicit errors instead of silently dropping Undo.

## Status

Design complete; no runtime changes. Implementation should start with durable action receipts, then Undo persistence/execution, then desktop routing and targeted event reconciliation. See HOSTED-SERVER-054 in `TASKS.md`.
