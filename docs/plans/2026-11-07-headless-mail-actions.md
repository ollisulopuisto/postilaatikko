# Headless Mail Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expose authenticated, idempotent archive, read/unread, star/unstar, trash, and move actions through the always-on localhost mailbox API.

**Architecture:** Reuse `internal/automation/v1.ActionsHandler` and its `operations.Target` resolver. Add a runtime adapter backed by the helper's existing message/folder stores, sync engine, IMAP pool, and cross-account copy-before-cleanup mover; do not open another database or sync owner. Provider writes must be confirmed before reporting success, with account-qualified targets validated before mutation.

**Tech Stack:** Go, `internal/automation/v1`, `internal/operations`, `internal/mailmove`, existing Gmail and IMAP clients, SQLite stores.

---

### Task 1: Claim and scope the action slice

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-07-headless-mail-actions.md`

**Steps:**
1. Mark HOSTED-SERVER-009 integrated at merge `494b26f` and claim HOSTED-SERVER-010 on its own branch/worktree.
2. Preserve the task's full action surface and its idempotency, capability, account-isolation, and safe-move acceptance criteria.

### Task 2: Implement headless provider mutations

**Files:**
- Create: `internal/alwayson/actions_api.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `internal/mailmove/filter_imap.go`
- Modify: `internal/automation/v1/actions.go`
- Reuse: `internal/operations/mail.go`

**Steps:**
1. Implement `v1.ActionOperations` against the runtime's existing stores and provider clients; use `operations.Resolve` for explicit account-qualified message/thread targets.
2. Apply read/unread and star/unstar state through Gmail `Modify` or selected-folder IMAP flags; persist local flags only after provider confirmation.
3. Validate the requested destination folder and every source message before a move. Use Gmail label changes for Gmail-native moves, and IMAP operations for IMAP accounts.
4. For cross-account moves, append/confirm every destination copy before touching the source. Reuse `mailmove.CopyBeforeCleanup` and the helper's existing IMAP mover; on any copy failure, preserve the source.
5. Implement archive and trash by resolving the account's provider-appropriate special folder, then route through the same safe move path.
6. Return the existing operation receipt and resolved message IDs. Keep one `ActionsHandler` mounted for the API lifetime so idempotency replays remain stable.

### Task 3: Mount, document, and verify the API

**Files:**
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `docs/API.md`
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`

**Steps:**
1. Mount `/v1/actions` alongside the authenticated read routes using `automation.RequireGrantCapability`; the handler requires `mail.write`.
2. Document the action schema, required `Idempotency-Key`, supported actions, account-qualified targets, and safe move behavior.
3. Run `gofmt`, `git diff --check`, and focused Go builds for the app, helper, and affected packages. Do not run tests locally per the active project instructions; rely on required GitHub CI for the full suite.
4. Commit the implementation and review-status ledger update, push the task branch, and open a PR without merging.

### Safety invariants

- Reject missing, duplicate, stale, or account-mismatched message references before mutation.
- Reject missing, stale, or mismatched destination folders before mutation.
- Never delete/move a source copy before a cross-account destination copy is confirmed.
- Do not fetch message bodies as part of an action.
- Keep action routes behind the existing authenticated server and capability gate.
