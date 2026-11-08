# HOSTED-SERVER-057: Authenticated Desktop Action and Undo Adapter

## Goal

Make connected local-helper and remote desktops use the same helper-owned action and Undo contract. Keep local Undo/provider behavior only when the localhost helper is unavailable; remote authorization and transport errors must fail closed.

## Scope

- Add automation client methods and typed results for helper Undo history, latest, and by-operation routes. Generate stable per-request Undo keys and retain them after ambiguous timeouts.
- Route `Undo`, `UndoLatestWithResult`, `UndoOperation`, `CanUndo`, and `GetUndoHistory` through the configured helper whenever connected, including local-helper mode. Do not use the desktop Undo stack in that path.
- Add a capability-checked target-resolution endpoint so account-qualified refs/threads from remote service results are validated by the helper.
- For remote actions, retain the helper-validated refs associated with the action selection and send those refs to the helper; do not read local mailbox rows or provider credentials. Keep local mode and helper-unavailable fallback behavior intact.
- After remote/helper Undo, emit the existing refresh event and return the helper operation ID/state needed by current UI callers.

## Safety boundaries

- Never fall back to local state after a connected helper returns an auth, capability, network, or action error.
- Helper response `outcome_unknown` is an explicit ambiguous result, not success/failure that permits a new key.
- Undo/latest requests use one idempotency key per user action; retrying a timed-out latest Undo must not undo an older operation.
- Remote-only startup has no usable local mailbox DB, provider pool, or credential store.

## Acceptance

1. Local-helper and remote clients can read bounded Undo history and invoke latest/by-operation Undo through authenticated client methods.
2. Undo keys persist across ambiguous retries and are replaced only after a terminal response.
3. Remote target resolution and mutation use account-qualified references returned by the helper, with no desktop message-store/provider access.
4. Connected helper errors fail closed; local fallback remains limited to helper-unavailable localhost mode.
5. Current UI Wails signatures remain compatible, tests/build checks appropriate to the touched packages pass, and `git diff --check` is clean.

## Implementation sequence

1. Add client methods and connector idempotency state for Undo calls.
2. Add helper-backed Undo/history branches to App methods.
3. Add authenticated target resolution and remote action reference flow.
4. Guard event publication and local fallback boundaries; update task ledger with limitations.
