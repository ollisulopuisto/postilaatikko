# Hosted Server Virtual Folder Ownership Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the helper the owner of persisted virtual-folder operations while preserving desktop behavior and local fallback only when the helper is unavailable.

**Architecture:** Extend authenticated saved-view reads and add `mail.write` mutation endpoints backed by the helper runtime's existing virtual-folder store and cache refresher. Extend the desktop API client and connector so Wails virtual-folder reads and mutations use those endpoints when connected; authorization, validation, and server errors must reach the caller. Keep search behavior on its existing path for this slice.

**Tech Stack:** Go, `net/http`, SQLite stores, Wails bindings.

---

### Task 1: Define the task boundary and API contract

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-hosted-server-virtual-folder-ownership.md`
- Inspect: `internal/automation/v1/actions.go`, `internal/automation/client/client.go`, `app/mailbox_service.go`, `internal/alwayson/sync.go`

**Steps:**
1. Claim HOSTED-SERVER-026 in `TASKS.md` before implementation.
2. Confirm the API can represent virtual-folder create, update, delete, reorder, and cache refresh using the helper's existing store.
3. Keep this task to persisted virtual-folder operations; defer filter/triage/draft ownership to later tasks.
4. Commit the task claim and plan separately before code changes.

### Task 2: Add authenticated virtual-folder mutation routes

**Files:**
- Create or modify: `internal/automation/v1/virtual_folders.go`
- Modify: `internal/automation/v1/types.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: helper API composition and its route contract documentation.

**Steps:**
1. Define typed request/response payloads for create/update/delete/reorder/cache refresh.
2. Mount routes under `/v1/views` and require `mail.write` for every mutation.
3. Validate method, body size, IDs, and operation-specific fields; return the existing stable API error envelope.
4. Implement operations against `SyncRuntime.virtualFolders`; refresh the service-owned cache after create/update and expose an explicit refresh operation.
5. Preserve server-side validation and avoid returning credential or message-body data.

### Task 3: Add desktop client and service connector calls

**Files:**
- Modify: `internal/automation/client/client.go`
- Modify: `app/mailbox_service.go`
- Modify: the virtual-folder API methods in `app/virtual_folder.go`.

**Steps:**
1. Add client methods matching the mutation wire contract.
2. Extend the mailbox service connector with typed calls and propagate non-availability errors.
3. Route `CreateVirtualFolder`, `UpdateVirtualFolder`, `DeleteVirtualFolder`, and `ReorderVirtualFolders` through the helper when connected.
4. Keep local store behavior only when the helper is unavailable; do not fall back on permission, validation, or server errors.
5. Leave virtual-folder reads/search on existing behavior in this task.

### Task 4: Document behavior and review

**Files:**
- Modify: `docs/API.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `TASKS.md`

**Steps:**
1. Document mutation routes, required `mail.write` capability, and desktop fallback conditions.
2. Update the ownership plan to mark the virtual-folder operation slice complete and list remaining workflow stores.
3. Run `gofmt` and `git diff --check`; build only the affected Go packages.
4. Record verification and known limitations in the task row, commit the implementation, and push the task branch.

Automated tests are intentionally excluded for this session; do not claim live multi-process validation.
