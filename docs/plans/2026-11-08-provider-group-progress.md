# Provider Group Progress Implementation Plan

> **Implementation note:** Follow the repository task/worktree instructions. Keep each mutation's idempotency reservation and provider-group journal in the helper database.

**Goal:** Make multi-account helper actions report durable confirmed and uncertain provider-group outcomes, allow Undo only for confirmed groups, and reconcile affected mailbox state without replaying ambiguous effects.

**Architecture:** Resolve and snapshot all targets before effects, partition them into deterministic provider groups, and persist the complete group plan in the same transaction as the operation reservation. Execute each group once, checkpoint `processing` before provider I/O and `confirmed` or `outcome_unknown` afterwards, and retain only confirmed groups in the Undo snapshot. Persist group states independently of a full response so helper restarts can reconstruct an accurate partial receipt and targeted event.

**Tech Stack:** Go, `internal/automation/v1`, `internal/alwayson`, SQLite migration 75, action receipts/events, helper-owned Undo.

---

## Task 1: Define provider group contracts and deterministic planning

**Files:**
- Modify `internal/automation/v1/actions.go` for content-free group and outcome types plus optional group-operation interfaces.
- Modify `internal/alwayson/actions_api.go` to plan action groups by account/provider/source folder and execute one planned group.

Partition flags by Gmail account or IMAP account+source folder. Partition moves by source account+folder and destination account/folder. Validate every group and destination before reserving. Never split a provider copy/delete move into independently retryable pieces that could delete a source before its destination copy is confirmed.

## Task 2: Persist planned and confirmed group state

**Files:**
- Modify `internal/database/migrations.go` with migration 75 and a `mail_action_groups` table.
- Modify `internal/automation/v1/actions.go` with an optional group receipt-store contract.
- Modify `internal/alwayson/action_receipt_ledger.go` to reserve all groups atomically, checkpoint each state, load group state for replay, and mark interrupted `processing` groups `outcome_unknown` at startup.

Store only operation/group IDs, account and folder IDs, helper message IDs, group state, and bounded undo data. No bodies, headers, raw idempotency keys, or provider credentials. Keep the existing receipt API compatible for non-production stores.

## Task 3: Execute groups once and report partial outcomes

**Files:**
- Modify `internal/automation/v1/actions.go` to execute planned groups with durable checkpoints and return `partial`, `confirmed`, and `outcome_unknown` group outcomes in the action response.
- Modify `internal/alwayson/action_receipt_ledger.go` so provider errors and restart recovery write event refs and receipts from the persisted group journal.
- Modify `internal/automation/v1/types.go` only if a shared operation-state constant is required; do not reinterpret send states.

Persist `processing` before each provider call. After any error, record that group as uncertain and continue independent groups. Never retry a group that is already processing/unknown. The overall response must distinguish full success, all-uncertain, and mixed partial outcomes.

## Task 4: Limit Undo to confirmed groups

**Files:**
- Modify `internal/alwayson/action_undo.go` to filter the pre-action snapshot to confirmed group message IDs.
- Modify `internal/alwayson/action_receipt_ledger.go` to expose Undo only when a nonempty confirmed-only snapshot is stored, including partial parent operations.

Unknown groups must never enter the Undo payload. Undo remains atomic and idempotent through the existing endpoints. Keep prior folder/flag guards intact.

## Task 5: Reconcile, verify, and integrate

**Files:**
- Modify action client/app adapters only if required for typed partial response fields to reach the UI without turning a known partial result into a generic transport failure.
- Update `TASKS.md` with verification and commit/integration status.

Use `gofmt`, `git diff --check`, and focused app/helper builds. Do not add or run tests under the current session instruction. Inspect the final SQL/action diff for atomicity, privacy, destination-copy-before-source-delete, and no-replay behavior before integration.

---

## Verification evidence

- A reservation contains the full deterministic group plan before the first provider effect.
- Each group transition is durable; startup converts stranded `processing` groups to `outcome_unknown` and never schedules them again.
- Responses and action events include partial state, per-group outcome, and bounded affected refs without mail content.
- Partial Undo contains only confirmed message snapshots; unknown groups remain excluded.
- App/helper packages build and `git diff --check` passes. Automated tests are omitted per the higher-priority session instruction.
