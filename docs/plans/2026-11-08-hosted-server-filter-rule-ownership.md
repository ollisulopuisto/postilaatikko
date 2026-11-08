# Hosted Server Filter Rule Ownership Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the helper the write owner for persisted automatic filter rules while preserving the existing safe move pipeline.

**Architecture:** Add authenticated filter-rule API operations backed by the helper runtime's existing `mailfilter.Store`; validate source-account and destination-folder invariants against service-owned account/folder stores. Extend the desktop API client and route Wails rule list/mutations through the helper when it is connected and write-authorized, retaining local behavior only when the helper is unreachable.

**Tech Stack:** Go, `net/http`, SQLite stores, Wails bindings.

---

### Task 1: Claim the work and map existing rule behavior

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-hosted-server-filter-rule-ownership.md`
- Inspect: `app/filter.go`, `internal/mailfilter/store.go`, `internal/alwayson/catalog_api.go`

**Steps:**
1. Claim HOSTED-SERVER-027 before code changes.
2. Preserve the current destination/source-account validation and rule priority behavior.
3. Keep safe message moves, job recovery, and uncertain-job endpoints out of this task.
4. Commit the claim and plan before implementation.

### Task 2: Add authenticated filter-rule routes

**Files:**
- Create: `internal/automation/v1/filter_rules.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: helper runtime filter operations as needed.

**Steps:**
1. Add typed list/create/update/delete/reorder wire operations.
2. Require `mail.read` for listing and `mail.write` for mutations.
3. Validate destination existence, source account existence, and cross-account constraints in the service.
4. Keep all writes on the helper runtime's existing rule store, which is also read by the existing automatic router.

### Task 3: Route desktop rule operations through the API

**Files:**
- Modify: `internal/automation/client/client.go`
- Modify: `app/mailbox_service.go`
- Modify: `app/filter.go`

**Steps:**
1. Add typed client and connector methods for the rule operations.
2. Route list and mutations through the helper when connected.
3. Require `mail.write` for rule mutations; surface permission and API errors.
4. Fall back to the existing local validation/store path only when the helper is unreachable.

### Task 4: Document and hand off

**Files:**
- Modify: `docs/API.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `TASKS.md`

**Steps:**
1. Document the filter rule routes and capability gates.
2. Update the ownership plan to identify rule CRUD as complete and job inspection/recovery as remaining work.
3. Run `gofmt`, `git diff --check`, and builds for affected Go packages. Automated tests are excluded for this session.
4. Record verification and limitations, commit, and push the task branch.
