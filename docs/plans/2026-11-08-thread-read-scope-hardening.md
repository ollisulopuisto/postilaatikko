# Thread Read Scope Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep thread context messages visible without letting body refreshes or automatic read-on-leave expand mailbox actions beyond the selected view, and preserve a private, account-safe authenticated thread response.

**Architecture:** Treat the service `Matched` bit as view-scope metadata that survives device-side body replacement and controls automatic read candidates. Resolve a thread reference as either a canonical account-qualified thread ID or an older message ID whose stored thread has since changed. Test the serialized read contract using populated message and attachment data so private body and path fields stay out of the response.

**Tech Stack:** Go, SQLite, authenticated v1 thread API, Svelte/TypeScript, Node test runner.

---

### Task 1: Preserve scoped message semantics in the reader

**Files:**
- Modify: `frontend/src/lib/components/viewer/ConversationViewer.svelte`
- Modify: `frontend/src/lib/components/viewer/pendingReadOnLeave.ts`
- Modify: `frontend/tests/pendingReadOnLeave.test.mjs`
- Create: `frontend/src/lib/components/viewer/threadScope.ts`
- Create: `frontend/tests/threadScope.test.mjs`

**Steps:**
1. Extract a small helper that applies fetched body/content fields while retaining the original message's `matched` scope marker.
2. Use that helper in `fetchUnfetchedBodies` before replacing the displayed message.
3. Make `PendingReadOnLeave.capture` ignore explicit unmatched context messages while retaining current local-message behavior when scope metadata is absent.
4. Add focused tests for unmatched body-refresh replacements and for unread context messages never entering the pending read set.

### Task 2: Canonicalize stale thread references safely

**Files:**
- Modify: `internal/message/store.go`
- Modify: `internal/message/thread_test.go`
- Modify: `internal/alwayson/mail_read_api.go`
- Create: `internal/alwayson/mail_read_api_test.go`

**Steps:**
1. Add an account-qualified store lookup that resolves either a current thread ID or a message row ID to its current thread ID.
2. Make `GetThread` use the canonical value for message lookup, returned `ThreadView.Ref`, and scope matching.
3. Add store cases for a stale message-ID reference and for rejecting resolution across accounts.
4. Add a `GetThread` fixture containing matched/unmatched messages and an attachment with a private local path and bytes; assert required headers and attachment summaries are present, while serialized output omits body text/HTML, local paths, and attachment bytes.

### Task 3: Verify, document, and record

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`

**Steps:**
1. Add a release-facing note for the corrected thread scope and safe metadata-only response.
2. Run focused Go and frontend regression tests for the changed behavior, then `git diff --check` and focused app/helper builds.
3. Update the ownership protocol and task row with commit, verification, and remaining limits; commit the task branch.
