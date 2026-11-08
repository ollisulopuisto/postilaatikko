# Hosted Server Triage Ownership Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the helper the persistence owner for Read Later membership and Snooze deadlines, including expiration when the desktop is closed.

**Architecture:** Add authenticated triage list/count/mutation endpoints backed by the helper runtime's existing triage store. Reuse helper-local Gmail label handling for Read Later when modify scope exists. Start a small maintenance loop independently of mailbox sync so expired Snooze rows are removed even when sync is disabled. Route desktop reads and changes through the helper while it is connected, preserving local fallback only for helper unavailability and retaining desktop events.

**Tech Stack:** Go, `net/http`, helper-local OAuth credentials, SQLite stores, Wails bindings.

---

### Task 1: Claim and map current behavior

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-hosted-server-triage-ownership.md`
- Inspect: `app/triage.go`, `internal/triage/store.go`, helper runtime lifecycle.

**Steps:**
1. Claim HOSTED-SERVER-030 before implementation.
2. Preserve idempotent Read Later membership, Gmail label behavior, snooze validation, count/list semantics, and expiry events.
3. Keep Reply Later and draft/send operations outside this task.
4. Commit the claim and plan before code changes.

### Task 2: Add helper-owned authenticated triage operations

**Files:**
- Create: `internal/automation/v1/triage_http.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `internal/alwayson/sync.go`
- Modify: `internal/scheduledsend/smtp_transport.go`
- Modify: `cmd/postilaatikko-helper/main.go`

**Steps:**
1. Add list/count and set-membership/snooze/unsnooze routes with `mail.read` and `mail.write` gates.
2. Back each route with the service runtime's existing stores and helper-local Gmail triage label method.
3. Add service-owned periodic snooze expiry that runs even when background mail sync is disabled.

### Task 3: Route desktop operations through the service

**Files:**
- Modify: `internal/automation/client/client.go`
- Modify: `app/mailbox_service.go`
- Modify: `app/triage.go`

**Steps:**
1. Add typed client/connector methods for Read Later and Snooze routes.
2. Route desktop reads and mutations through the helper when connected and authorized.
3. Preserve local fallback only for helper unavailability and keep local change/expiry events up to date.

### Task 4: Document and hand off

**Files:**
- Modify: `docs/API.md`
- Modify: `docs/plans/2026-11-06-always-on-mail-service.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `TASKS.md`

**Steps:**
1. Document API capabilities and helper-owned expiry behavior.
2. Run `gofmt`, `git diff --check`, and builds for affected Go packages. Automated tests are excluded for this session.
3. Record verification and live Gmail limitations, commit, and push the task branch.
