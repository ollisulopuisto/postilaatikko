# Desktop Global Search Service Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make desktop global search use the authenticated mailbox service while preserving the existing Wails response shape, pagination, account scope, partial failures, and cancellation.

**Architecture:** Extend the shared view/search contract with all-account and single-account query scopes, and include the folder/account details the desktop needs in each thread summary. The app adapter calls the existing `/v1/search` endpoint when connected, translates its response into `MailSearchPage`, and cancels the HTTP request when its UI session is superseded. Local search remains available when the helper is unavailable; credential, capability, and request errors remain visible.

**Tech Stack:** Go, `internal/views`, authenticated `internal/automation/v1` API/client, Wails app adapter, markdown API contract.

---

### Task 1: Extend API search scope and result metadata

**Files:**
- Modify: `internal/views/model.go`
- Modify: `internal/automation/v1/types.go`
- Modify: `internal/automation/v1/read.go`
- Modify: `internal/alwayson/mail_read_api.go`
- Modify: `internal/automation/client/client.go`
- Modify: `docs/API.md`

Add account and all-account scopes with validation and stable keys. Carry the existing mail search filter through `SearchRequest`. Preserve account name, selected folder, and reauthorization state in `ThreadPage` results so the app adapter can reconstruct its current model without extra provider requests. Keep cursors and result pages bounded.

### Task 2: Route and cancel desktop global search

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `app/mail_search.go`
- Modify: `app/app.go` (or the existing app field declaration)

For supported global-search requests, convert query/account/filter/page data into a typed service request, map summaries and partial errors into `MailSearchPage`, and track request cancellation by session ID. Preserve virtual-folder and other unsupported search paths. Fall back locally only for an unavailable helper; propagate authorization and capability failures.

### Task 3: Verify and record

Run `gofmt`, `git diff --check`, the focused Go build and changed-package lint. Do not run local tests in this task environment. CI on the PR provides the repository test suite. Update `TASKS.md` with the implementation commit and review status before handoff.
