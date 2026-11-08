# Hosted Folder Ownership Plan

**Goal:** Make the helper the owner of provider folder discovery and subscription changes while keeping existing Gmail and IMAP behavior.

**Architecture:** Reuse the authenticated `/v1/folders` catalog read, add helper operations to synchronize folders from a provider and set one account-qualified folder's subscription state, and route the desktop folder UI through those operations when the helper is connected. The helper uses its local OAuth/IMAP credentials. Local operations remain available only while the helper is unreachable; authorization and service errors are returned to the caller.

**Tech Stack:** Go, `net/http`, existing folder store and sync engine, helper-local credentials, Wails service connector.

## Task 1: Claim and map folder behavior

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-hosted-folder-ownership.md`
- Inspect: `app/folder.go`, `internal/sync/folders.go`, `internal/folder/store.go`, helper API composition.

**Steps:**
1. Claim HOSTED-SERVER-032 before implementation.
2. Preserve folder ordering, empty-folder discovery on mapping screens, Gmail label visibility, IMAP SUBSCRIBE/UNSUBSCRIBE, and account/folder ownership checks.
3. Commit the claim and plan before code changes.

## Task 2: Add authenticated helper folder operations

**Files:**
- Create: `internal/automation/v1/folder_mutations.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `internal/automation/client/client.go`
- Modify: `app/mailbox_service.go`
- Modify: `app/folder.go`

**Steps:**
1. Add `mail.write` routes for account-qualified provider folder sync and subscription changes. Verify the requested folder belongs to the requested account.
2. Keep `/v1/folders` as the service-owned catalog read and add typed client/connector methods for sync and subscription.
3. Route desktop folder reads and changes through the helper when connected and write-authorized; use local operations only when it is unavailable.

## Task 3: Document and review

**Files:**
- Modify: `docs/API.md`, `docs/plans/2026-11-08-service-database-ownership-protocol.md`, `CHANGELOG.md`, `TASKS.md`.

**Steps:**
1. Document the routes, capability checks, and helper-local credential behavior.
2. Run `gofmt`, `git diff --check`, and focused Go builds. Do not run tests unless requested.
3. Record verification and limitations, commit, push for review, and integrate through the coordinator.
