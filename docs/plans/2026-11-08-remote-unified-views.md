# Remote Unified Views Implementation Plan

> **For Claude: REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.**

**Goal:** Let remote-only desktops browse, count, search, and open conversations in the supported unified system folders without reading local mailbox rows.

**Architecture:** Add a validated `unified_folder` scope to the shared view contract and authenticated bounded list/count/search APIs. The helper resolves special folders for enabled accounts and performs archive merging/exclusion from its authoritative stores; the desktop sends only the semantic folder type and routes list, count, search, and thread scope through the helper. Keep physical folder reads and local-helper behavior unchanged.

**Tech Stack:** Go, SQLite-backed message/folder stores, authenticated v1 API, Wails app adapter, TypeScript shared scope contract.

---

### Task 1: Extend the shared scope contract

**Files:**
- Modify: `internal/views/model.go`.
- Modify: `frontend/src/lib/views/types.ts`.
- Modify: `internal/views/testdata/scopes.json`.

Add `ScopeUnifiedFolder` with a `folderType` field. Validate only the supported system types (`drafts`, `sent`, `trash`, `starred`, `archive`, `spam`); Inbox remains the existing `all_inboxes` scope. Reject all unrelated fields and include the type in `ScopeKey`. Mirror the shape in TypeScript and shared fixtures.

### Task 2: Add helper-owned bounded list and count operations

**Files:**
- Modify: `internal/automation/v1/conversation_list.go` and `conversation_count.go`.
- Modify: `internal/alwayson/catalog_api.go`.

Resolve requested special folders from enabled accounts on the helper. Validate the resulting folder/account ownership before querying. For Archive, merge physical Archive folders and Gmail-style All Mail rows excluding Inbox copies, globally sort, then apply offset/limit; count from the same two sources without loading all rows. Return account and folder identities on every conversation.

### Task 3: Add unified-folder search and thread matching

**Files:**
- Modify: `internal/message/store.go`.
- Add: `internal/automation/v1/conversation_search.go`.
- Modify: `internal/alwayson/catalog_api.go` and `internal/alwayson/mail_read_api.go`.

Search the helper's indexed headers, matching the existing in-folder search behavior. Resolve unified folders through helper stores; Archive search merges physical Archive results with virtual All Mail results and excludes Inbox copies in the SQL query. Extend `messageInScope` so thread reads report the messages that actually match the selected unified folder. Preserve bounded pagination and exact counts.

### Task 4: Route remote desktop unified views through the service

**Files:**
- Modify: `app/unified.go` and `app/mailbox_service.go`.
- Modify: `CHANGELOG.md` and this plan's status.

Route remote list/count calls through the authenticated conversation routes with `ScopeUnifiedFolder`, and in-folder search through the authenticated indexed-search route. Remove the Inbox-only rejection for supported types. Keep local mode paths unchanged and return clear unsupported errors for queue views that use separate triage APIs. The helper remains the background sync owner; remote manual folder sync is reported as unavailable.

### Task 5: Verify and record

Run `gofmt` on changed Go files, `git diff --check`, and `GOCACHE=/private/tmp/postilaatikko-go-cache GOPROXY=off go build ./app ./cmd/postilaatikko-helper`. Do not run tests in this session. Record frontend verification availability and remaining live-provider limitations in `TASKS.md`.

## Limits

This slice covers unified system folders only. Reply Later, Read Later, Snoozed, and other triage queues remain their own service operations. Remote manual folder sync is still unavailable; the helper owns periodic sync. Live Gmail and IMAP parity is not established by compilation.

## Status

Implementation is complete. The Go app/helper build and `git diff --check` pass. Tests were not run in this session, frontend dependencies are absent, and live-provider/runtime behavior remains a follow-up verification.
