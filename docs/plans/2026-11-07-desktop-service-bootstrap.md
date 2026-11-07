# Desktop Local Service Connection and Body Fetch Plan

**Goal:** Establish an authenticated desktop connection to the localhost mailbox service and route explicit message-body fetches through it when connected.

**Architecture:** Reuse the v1 HTTP client and SQLite API token store. Keep the desktop bearer token in the OS keyring and provision only `mail.read` and `mail.body.fetch`. Validate both the stored grant and the service's negotiated capabilities before reporting a connected state. The helper handles explicit account-qualified body fetches; the desktop reads the resulting shared database row and performs its normal reaction enrichment. Use the existing local body-fetch path only when the helper is unreachable. Authorization, credential, and capability errors must stop the request without fallback. Routine listing and sync stay header-only.

**Tech stack:** Go, Wails, SQLite, `internal/automation/client`, `internal/automation`, `internal/keyring`.

## Task 1: Add a generic OS-keyring secret slot

**Files:** `internal/keyring/keyring.go`

Add `SetSecret`, `GetSecret`, and `DeleteSecret` methods using the existing service namespace and error redaction. Keep password and OAuth helpers unchanged.

## Task 2: Provision and retain the desktop API grant

**Files:** `app/mailbox_service.go`, `app/app.go`

At startup, load the persisted desktop client record from the keyring. If no record exists and no desktop grant has ever been issued, register a client with only `mail.read` and `mail.body.fetch` and persist its client ID and bearer token in the keyring. Revoke the new grant if the keyring write fails. If an existing grant is revoked, missing, malformed, or has different capabilities, report that authorization needs repair; do not silently mint a replacement. Never log the bearer token or serialize it into Wails output.

## Task 3: Expose connection status through Wails

**Files:** `app/mailbox_service.go`, `app/app.go`

Use the fixed loopback endpoint `http://127.0.0.1:8029`. Check the service with a short timeout and report reachability, API version, granted capabilities, and a safe error code/message. Do not include the token, secret, or raw HTTP response. Service unavailability must not block desktop startup.

## Task 4: Route explicit body fetches through the helper

**Files:** `app/message.go`, `app/mailbox_service.go`

For a user-requested message body, resolve the message's account ID and call `POST /v1/messages/body` with `Mode: fetch` and `includeOriginal: true` when the service is connected. Read back the updated shared-database message and retain the desktop's existing reaction enrichment. If the helper is unreachable, use the established local fetch path. Do not fall back for authorization, credential, API, or capability failures. Do not add body fetching to list or sync paths.

## Task 5: Document the service boundary

**Files:** `docs/API.md`, `TASKS.md`

Document the least-privilege grant, keyring storage, body-fetch handoff, unreachable-helper fallback, and remaining desktop paths. Record verification and the live-service limitation in the task row.

## Verification

- Run `gofmt` on changed Go files.
- Run `go build ./app ./internal/automation/...` from the task worktree.
- Run `git diff --check`.
- Do not run or add tests unless separately requested.

## Commit

Commit the task implementation on `agent/HOSTED-SERVER-012` after reviewing the worktree status.
