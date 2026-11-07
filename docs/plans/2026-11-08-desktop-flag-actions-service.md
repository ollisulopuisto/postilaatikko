# Desktop Flag Actions Through the Mailbox Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Route desktop read/unread and star/unstar actions through the authenticated mailbox service after the desktop has explicit `mail.write` authorization.

**Architecture:** Keep the existing Wails action methods and UI contracts. Add a deliberate authorization step for the desktop service credential; never expand an existing read-only grant or silently replace its token. When the service is connected and the grant has `mail.write`, send account-qualified message targets with an idempotency key, then emit the desktop's existing UI events. Fall back to the existing local path only when the helper is unavailable; surface authorization and service errors.

**Tech Stack:** Go Wails app, authenticated automation client, SQLite grant store, existing operations target contract, Svelte settings UI.

---

### Task 1: Add an explicit desktop write-authorization path

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `internal/automation/sqlite_token_store.go` only if grant replacement needs a narrow helper
- Modify: `frontend/src` service settings or connection UI
- Modify: generated Wails bindings if required

Expose a user-initiated action that creates a desktop grant with `mail.read`, `mail.body.fetch`, and `mail.write`, saves the new credential in the OS keyring, then revokes the old desktop grant. Preserve the old token and grant if creating or saving the new credential fails. Keep status output token-free and report missing write authorization distinctly from helper unavailability.

### Task 2: Route read and star flag changes

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `app/actions.go`
- Modify: generated Wails bindings only if the exposed status or authorization model changes

Build account-qualified message refs from the existing local message store. Send `mark_read`, `mark_unread`, `star`, and `unstar` actions through `Client.MutateAction` with a fresh idempotency key. Preserve desktop event and folder-count updates after service success. If the helper is unavailable, use the current local mutation path; do not fall back for revoked grants, missing `mail.write`, or API failures.

### Task 3: Verify integration and record the task

Run formatting, focused Go build and changed-package lint. Let CI run the repository checks; do not add or run local tests in this task environment. Confirm the authorization flow does not overwrite a credential on failure and that the four actions preserve existing frontend event behavior. Update `TASKS.md` with implementation and review status.
