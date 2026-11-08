# Desktop Service Move Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Route desktop archive, trash, and folder moves through the authenticated mailbox service while preserving durable Undo and safe move semantics.

**Architecture:** Keep the authenticated `/v1/actions` endpoint as the provider-operation boundary. For single-account operations, the desktop captures account-qualified message identities and source/destination folders; after the service confirms success, it registers the existing durable `undo.MoveCommand` entries with the service receipt's operation ID. Undo uses the same authenticated service action path when connected and retains local behavior only when the helper is unavailable. Multi-account actions remain on the proven desktop path until the API has explicit partial-completion receipts and Undo supports distinct source and destination accounts.

**Tech Stack:** Go, SQLite-backed mail and undo stores, Wails application bindings, authenticated mailbox API client.

---

### Task 1: Add the service move adapter and durable Undo registration

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `app/actions.go`
- Reuse: `internal/undo.MoveCommand`, `undo.OperationStore`, and existing operation-scoped mutation guard

**Steps:**
1. Add a helper that checks connection and `mail.write` authorization using the existing service-status rules and submits an account-qualified `ActionRequest` with an idempotency key.
2. Snapshot source message/folder identities before dispatch; use the desktop's already-resolved archive/Trash folder ID so account folder mappings keep their current meaning.
3. After a successful receipt, group snapshots by source and destination folder, create the existing serializable `MoveCommand` entries, and push them with the response operation ID and appropriate action metadata.
4. Return service errors as errors. Use local fallback only for the existing helper-unavailable status, and never retry locally after a service action may have succeeded.
5. Preserve existing runtime events, folder counts, and mutation coalescing semantics around the new route.

### Task 2: Route archive, Trash, and explicit moves through the service

**Files:**
- Modify: `app/actions.go`
- Reuse: `app/mailbox_service.go`

**Steps:**
1. Add service-first routing to the shared same-account move path; archive, Trash, and explicit folder moves all use the same account-qualified `move` action and destination ID.
2. Keep multi-account archive/Trash and cross-account explicit moves on the existing desktop path until the API can report partial completion and Undo stores both source and destination accounts.
3. Ensure local move code is called only if the helper is unavailable; authorization, capability, and operation errors must surface without a duplicate local mutation.
4. Keep zero-message behavior and current folder-count/message-change event behavior intact.

### Task 3: Route Undo move mutations through the authenticated service

**Files:**
- Modify: `app/undo.go`
- Modify: `app/mailbox_service.go`

**Steps:**
1. In the UndoContext move adapter, submit an account-qualified service `move` action when connected and write-authorized.
2. Preserve the current local move path only when the helper is unavailable; propagate revoked-grant, authorization, and server-side move failures.
3. Confirm the existing single-use operation claim and durable command rehydration still apply when Undo crosses the service boundary.

### Task 4: Review implementation and update the task ledger

**Files:**
- Modify: `TASKS.md`

**Steps:**
1. Review action call paths for accidental provider fallback after an uncertain service result, missing account identity, and duplicate operation registration.
2. Run focused Go builds for the changed packages and `git diff --check`; do not run automated tests unless requested.
3. Record verification, limitations, and the implementation commit in the task row, then commit the task branch for review.
