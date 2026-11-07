# Shared Mail Read Operations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make desktop and headless API search and message reads use shared mailbox operations, then expose authenticated search, thread, and body read routes through the helper.

**Architecture:** Extract provider-aware search and message read behavior from the Wails `app.App` into reusable internal operation services. Keep current desktop methods as compatibility adapters and construct those same services from the headless runtime's existing stores and sync engine. Mount the full v1 read surface with capability checks; search stays header-only and body fetch is always explicit.

**Tech Stack:** Go, SQLite-backed account/folder/message stores, existing Gmail and IMAP sync clients, `internal/views` and `internal/automation/v1` contracts.

---

### Task 1: Extract the existing provider-aware search operation without changing desktop behavior

**Files:**
- Create: `internal/mailservice/service.go`
- Create: `internal/mailservice/search.go`
- Create: `internal/mailservice/types.go`
- Modify: `app/mail_search.go`
- Modify: `app/app.go`

**Steps:**
1. Move the provider-agnostic request/page/cursor types and search implementation into `internal/mailservice`; keep compatibility aliases in `app` for Wails and existing callers.
2. Inject account, folder, virtual-folder, queue, message, and sync dependencies explicitly. Preserve Gmail query construction, IMAP folder continuation, cancellation, account/folder-bound cursors, deduplication, partial failures, and header-only results.
3. Make `App.SearchMail` and `App.CancelMailSearch` delegate to the service while retaining public signatures and session cancellation behavior.
4. Run `gofmt` and `git diff --check`; build the app and affected service packages.

### Task 2: Mount authenticated search, thread, and body routes

**Files:**
- Modify: `internal/automation/v1/read.go` or split focused handlers into `internal/automation/v1/search.go` and `internal/automation/v1/messages.go`
- Modify: `internal/automation/server.go` only if route composition needs a mount point
- Create: `internal/alwayson/mail_read_api.go`
- Modify: `internal/alwayson/catalog_api.go` and `internal/alwayson/sync.go`
- Keep: `internal/automation/v1/types.go` and existing contract shapes unless evidence shows a mismatch

**Steps:**
1. Define narrow search and message-read interfaces that the helper can implement from the shared services; retain the existing combined handler for callers that provide the full contract.
2. Validate all-inbox, folder, saved-view, and search scopes; enforce account-qualified refs; map header results to `views.ThreadPage` without loading bodies.
3. Materialize scoped thread context from the local message store and expose its body completeness without serializing bodies.
4. Mount body reads; require `mail.body.fetch` for provider fetches and keep cached-only reads body-free when content has not been requested.
5. Map per-account search failures into visible `views.PartialError` entries and set completeness accurately when errors or continuations remain.
6. Run `gofmt`, `git diff --check`, and focused Go builds for the automation packages.

### Task 3: Wire the same service into the headless runtime and desktop API composition

**Files:**
- Create or modify: `internal/alwayson/search_api.go`
- Modify: `internal/alwayson/api_server.go`
- Modify: `internal/alwayson/sync.go` only if the runtime must expose an already-owned dependency
- Modify: `docs/API.md`, `CHANGELOG.md`, and `TASKS.md`

**Steps:**
1. Construct the read services from the runtime's existing database stores and credential-backed sync engine; do not open a second database or create a second sync owner.
2. Mount all read routes on the authenticated server and retain grant capability, Origin, Host, and loopback checks from the shared server.
3. Document provider-backed search, visible partial results, explicit body fetch, and actual route coverage; do not claim action/Pub/Sub routes are mounted.
4. Build the desktop app, helper, CLI, MCP and affected internal packages; run `git diff --check`; rely on PR CI for the repository test suite.

### Task 4: Review and publish

**Files:** `TASKS.md` and the branch changes above.

**Steps:**
1. Inspect the complete diff and worktree status; confirm only HOSTED-SERVER-009 files changed.
2. Commit the implementation and update the task row with commit, verification, and PR reference.
3. Push the task branch and open a review PR against the merged HOSTED-SERVER-008 work on `main`; leave the PR open for review.

**Current scope boundary:** This task wires search, thread, and body reads. Attachment list/read routes, mail mutations, and Gmail Pub/Sub reception/watch renewal remain separate follow-up slices. Background sync stays bounded and body fetch remains explicit.
