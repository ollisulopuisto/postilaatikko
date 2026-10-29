# Online Counts and IMAP Continuation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Resume the published GLOBAL-002 count/pagination checkpoint safely on current main, preserving saved-search and Gmail quota behavior.

**Architecture:** Restore server-enumerated Gmail count jobs and decreasing-UID IMAP pages. Keep counts distinct from loaded windows and report unsupported providers as incomplete. Reconcile the new saved query with counts and preserve cached first paint.

**Tech Stack:** Go, Gmail REST, go-imap/v2, Svelte 5, Wails.

### Task 1: Reconcile the checkpoint

**Files:** `app/view_count.go`, `app/mail_search.go`, `internal/gmail/count.go`, `internal/gmail/native.go`, `internal/sync/search_page.go`, `internal/sync/search.go`; count UI, generated bindings and locales.

1. Recover only committed changes from `e9f9be5`; preserve current main threading, quota budget, cache and saved queries.
2. Run checkpoint Go tests to confirm that recovery compiles on current main.

### Task 2: Guard current behavior with regression tests

**Files:** `app/view_count_test.go`, `internal/gmail/count_test.go`, `internal/sync/search_page_test.go`.

1. Add failing tests for saved-query count membership, duplicate scanned IDs and cancellation/account scoping as applicable.
2. Run focused tests and confirm failure.
3. Apply minimum fixes and confirm green.
4. Exercise multi-folder/account IMAP continuation and retry semantics with protocol fixtures.

### Task 3: Review and handoff

1. Run appropriate Go suites (including full `go test ./...`), frontend tests/check/lint/build and whitespace checks.
2. Record actual scope and remaining non-Gmail conversation-count gate in diagnostics and TASKS.md.
3. Commit and push the task branch for review, with no merge to main.
