# HOSTED-SERVER-069 Action Unknown Target Recovery

**Goal:** Preserve content-free account/folder/message references at helper action reservation so provider errors and helper restarts can produce targeted reconciliation events without replaying uncertain mutations.

## Design

- Add a message-reference JSON column to `mail_action_operations`, separate from the existing Undo payload and from event JSON.
- Add a receipt-store reservation extension that atomically stores the target refs with the operation intent. The helper must resolve and read refs before reserving or performing provider effects.
- Keep the original account-qualified target refs in the durable receipt. On confirmed success, return post-action refs when available so moves identify the new folder; on uncertain outcomes, return the reserved pre-action refs and rely on the `outcome_unknown` state to trigger a bounded refresh.
- Recover interrupted operations by copying the reserved refs into the existing action-event cursor record. Retries with a stored response preserve the same refs.
- Keep no message content or raw idempotency keys in the new field. Do not replay provider operations when the outcome is uncertain.

## Invariants

- A provider effect never begins before the helper has durably reserved its operation and account-qualified target refs.
- Any provider error or restart can identify which desktop rows need reconciliation.
- Existing successful message refs, event cursors, Undo snapshots, and action idempotency contracts stay compatible.
- No failed or ambiguous action is represented as confirmed success or made Undoable.

## Verification

- `gofmt`, `git diff --check`, and focused app/helper build only.
- No automated tests will be added or run under the current session instruction.

## Implementation

- Migration 74 adds `affected_messages_json` to helper action operations.
- Durable action reservations now store the resolved account/folder/message refs before undo capture or provider effects. The provider adapter must resolve complete refs before reservation; failure stops the action.
- Provider errors, failed undo-snapshot capture, and interrupted helper operations retain those refs in the action receipt/event. Successful operations return post-action refs when readable, with pre-action refs as a safe fallback.
- Verification: `gofmt`, `git diff --check`, and `GOCACHE=/private/tmp/postilaatikko-go-cache GOPROXY=off go build ./app ./cmd/postilaatikko-helper` pass. No tests were run.
