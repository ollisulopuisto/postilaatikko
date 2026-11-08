# HOSTED-SERVER-058: Action Result Reconciliation

## Goal

Keep the client cache-first and responsive while reconciling helper-owned actions from their bounded results. A helper action response should identify exactly which account/folder/message references changed, and unknown results must never be presented as confirmed success.

## Scope

- Add safe affected-reference summaries to action and Undo responses: account ID, message ID, and current folder ID only.
- Have helper operations collect these refs from helper-owned message rows after mutation/Undo; never include bodies, headers, or local paths.
- Forward refs through the Wails adapter and emit a typed reconciliation event.
- Update the mail-list/navigation stores to refresh only affected folders/threads and preserve the current virtual-folder cache-first page until the corresponding helper data arrives.
- Distinguish `sent`, `undone`, and `outcome_unknown` in UI feedback. Keep unknown operations in Undo/history and retry only with their existing operation key.
- Preserve fallback behavior for localhost-helper-unavailable mode and fail closed for remote service errors.

## Boundaries

- Do not trigger full-folder reloads or eagerly fetch message bodies/attachments.
- Group-level partial provider reconciliation remains a follow-up because the current action executor does not checkpoint every provider group.
- Avoid changing the virtual-folder background cache cadence or semantics in this slice; its existing helper cache remains the source for first paint.

## Acceptance

1. Action and Undo responses/events include only bounded account/folder/message references.
2. The client refreshes affected cached pages/counts and maintains current selection where possible.
3. Unknown outcomes show an explicit uncertain state and do not generate a success toast or a fresh idempotency key.
4. No full-folder/body/attachment fetch is added to the action-result path.
5. Relevant Go builds, frontend checks when dependencies are available, and `git diff --check` pass.

## Follow-up

Reconnect-time recovery of missed action notifications needs an operation cursor and a bounded event feed. This slice emits immediate reconciliation events and retains unknown operations in history; it does not replay action events missed while the desktop was closed. Track that as a separate helper API task.

## Implementation sequence

1. Extend helper and client result types with affected references.
2. Forward safe results through App/Wails and emit targeted events.
3. Connect existing list/count cache invalidation to those events and update action/Undo feedback for unknown results.
4. Record checks and remaining partial-group/restart limitations in `TASKS.md`.
