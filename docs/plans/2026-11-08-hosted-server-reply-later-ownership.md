# Hosted Server Reply Later Ownership Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the helper the owner of Reply Later membership and its Gmail label synchronization while keeping desktop Undo history and UI events intact.

**Architecture:** Add authenticated queue read and set-membership routes backed by the helper runtime's `replylater.Store`. Reuse helper-local Gmail credentials to update the same hidden label when the account has Gmail modify scope; local queue membership remains supported when that scope is absent or the account is not Gmail. The desktop calls the service when connected and write-authorized, registers its existing undo command after service success, and uses local persistence only when the helper is unreachable.

**Tech Stack:** Go, `net/http`, helper-local OAuth credentials, SQLite stores, Wails bindings.

---

### Task 1: Claim and map current behavior

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-hosted-server-reply-later-ownership.md`
- Inspect: `app/reply_later.go`, `app/triage.go`, `internal/replylater/store.go`, helper credential transport.

**Steps:**
1. Claim HOSTED-SERVER-029 before implementation.
2. Preserve current provider-before-store ordering, queue idempotency, scope behavior, and operation-scoped undo.
3. Keep Read Later, Snooze, and draft workflows outside this task.
4. Commit the claim and plan before code changes.

### Task 2: Add the authenticated helper API

**Files:**
- Create: `internal/automation/v1/reply_later_http.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `internal/scheduledsend/smtp_transport.go`
- Modify: `internal/alwayson/sync.go`
- Modify: `cmd/postilaatikko-helper/main.go`

**Steps:**
1. Add queue list and add/remove endpoints, gated by `mail.read` and `mail.write` respectively.
2. Implement helper runtime operations against `replylater.Store`.
3. Update the Gmail hidden label through helper-local credentials when Gmail modify scope exists; for non-Gmail or read-only grants, change only local queue membership as the desktop currently does.
4. Preserve account-qualified conversation IDs and return stable API errors.

### Task 3: Route desktop reads and changes

**Files:**
- Modify: `internal/automation/client/client.go`
- Modify: `app/mailbox_service.go`
- Modify: `app/reply_later.go`

**Steps:**
1. Add typed queue list/membership client calls.
2. Route queue reads and changes through the helper when connected and write-authorized.
3. Keep the existing Undo command on the desktop; create it only after a successful helper mutation and route Undo membership restoration through the helper too.
4. Preserve UI events; use local behavior only when the helper is unreachable.

### Task 4: Document and hand off

**Files:**
- Modify: `docs/API.md`
- Modify: `docs/plans/2026-11-06-always-on-mail-service.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `TASKS.md`

**Steps:**
1. Document capabilities, local credential use, and fallback behavior.
2. Run `gofmt`, `git diff --check`, and builds for affected Go packages. Automated tests are excluded for this session.
3. Record verification and live Gmail limitations, commit, and push the task branch.
