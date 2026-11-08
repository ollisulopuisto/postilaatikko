# HOSTED-SERVER-067 Triage Desktop Undo Plan

**Goal:** Make Read Later, Snooze, and Unsnooze desktop changes consume helper-owned receipts and Undo in both localhost and remote service modes, with reliable retry behavior and clear feedback.

## Scope

- Preserve the existing app API where it is already consumed; add result-bearing Wails methods where the current return type cannot carry an operation ID.
- Use stable request hashes and idempotency keys in the desktop helper connector for ambiguous retries, matching Reply Later.
- Route successful helper mutations to operation-scoped Undo and queue reconciliation. Never create a second desktop-memory Undo command for a helper-owned action.
- Permit direct local-store fallback only when the helper is explicitly unavailable. A connected helper's auth, validation, or network failures must fail closed.
- Remove remote-only local-store readiness checks from mutation and Undo flows.
- Give helper Undo history readable triage descriptions and make the existing success/error feedback identify the action.
- Keep the event cursor and queue refresh behavior added by HOSTED-SERVER-066.

## Implementation sequence

1. Inspect all Wails/frontend callers for Read Later, Snooze, and Unsnooze; identify existing local Undo behavior and generated binding conventions.
2. Add connector methods that call the existing `WithKey` helper client methods, keep stable keys across ambiguous outcomes, and expose operation receipts to the app layer.
3. Add app result APIs and operation routing for Read Later/Snooze/Unsnooze. Ensure helper paths use the helper receipt ID; local fallback behavior remains safe and does not pretend to have a helper receipt.
4. Update frontend stores and action menus to consume result receipts, keep immediate queue updates, and present understandable success/error and Undo history descriptions.
5. Remove any local triage-store prerequisites that still block remote operation or Undo; preserve explicit unsupported/error states.
6. Run `gofmt`, `git diff --check`, and the focused app/helper build. Do not add or run automated tests under this session instruction.

## Invariants

- Each helper mutation uses one stable idempotency key for retries of the same request and a new key for a different request.
- A helper-owned operation is never also pushed to desktop-memory Undo.
- Unknown helper outcomes remain visible/retryable and do not cause optimistic local writes.
- Local fallback occurs only when the helper is unavailable, never when it is connected but unauthorized or failing.
- Undo and queue identities remain account-qualified; Snooze restoration includes its previous deadline.
