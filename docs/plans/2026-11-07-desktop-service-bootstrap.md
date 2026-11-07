# Desktop Service Connection Bootstrap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Give the desktop a least-privilege, authenticated connection to the localhost mailbox service and expose whether that service is reachable.

**Architecture:** Reuse the existing v1 HTTP client and SQLite API token store. Keep the desktop bearer token in the OS keyring, grant only `mail.read`, and validate the negotiated API capabilities before reporting a connected state. This bootstrap does not route mailbox operations yet; the desktop continues its current behavior until a later task moves concrete reads and writes onto the service.

**Tech Stack:** Go, Wails, SQLite, `internal/automation/client`, `internal/automation`, `internal/keyring`.

---

### Task 1: Add a generic OS-keyring secret slot

**Files:**
- Modify: `internal/keyring/keyring.go`

Add `SetSecret`, `GetSecret`, and `DeleteSecret` methods that use the existing service namespace and error redaction. Keep the password and OAuth helpers behavior unchanged. Use a dedicated key name for the desktop service credential.

### Task 2: Provision and retain the desktop API grant

**Files:**
- Create: `app/mailbox_service.go`
- Modify: `app/app.go`
- Use: `internal/automation/sqlite_token_store.go`
- Use: `internal/keyring/keyring.go`

At startup, load the persisted desktop client record from the keyring. If no record exists and no desktop grant has ever been issued, register a `mail.read` client through the existing token store and persist the client ID and bearer token in the keyring. Revoke the newly created grant if the keyring write fails. If an existing grant is revoked, missing from the database, or malformed in the keyring, report that service authorization needs repair; do not silently mint a replacement. Never log the bearer token or serialize it into Wails output.

### Task 3: Expose connection status through Wails

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `app/app.go`
- Use: `internal/automation/client/client.go`

Use the fixed default loopback endpoint `http://127.0.0.1:8029`. Call `GetCapabilities` with a short timeout and report a typed status containing reachability, API version, granted capabilities, and a safe error code/message. Do not include the endpoint token, secret, or raw HTTP body. Service unavailability must not block desktop startup.

### Task 4: Document the bootstrap boundary

**Files:**
- Modify: `docs/API.md`
- Modify: `TASKS.md`

Document that the desktop grant is local, keyring-backed, and limited to `mail.read`; clarify that mailbox reads/actions still use the existing desktop path until follow-up handoff tasks integrate them. Record the changed files, checks, and live-service limitation in the task row.

### Verification

- Run `gofmt` on changed Go files.
- Run `go build ./app ./internal/automation/...` from the task worktree.
- Run `git diff --check`.
- Do not run or add tests unless separately requested.

### Commit

Commit the task implementation on `agent/HOSTED-SERVER-012` after reviewing the worktree status.
