# Helper OAuth Handoff Implementation Plan

> **For Codex:** Follow the repository task ledger and worktree instructions in `AGENTS.md`.

**Goal:** Make the always-on helper the owner of mailbox OAuth tokens, account-specific custom provider configuration, reauthorization persistence, and linked Microsoft shared-mailbox token refresh.

**Architecture:** Keep browser authorization and callback validation in the desktop for now. Add an authenticated, idempotent account-management operation that accepts the validated token package and optional custom provider definition, writes through the helper credential store, propagates token replacements to linked child accounts, and acknowledges only after the helper can use the new credentials. The desktop retains pending callback material on error and clears it only after acknowledgement; API errors, receipts, and logs contain no token or client-secret values.

**Tech Stack:** Go, `internal/automation/v1`, helper `SyncRuntime`, `internal/credentials.Store`, desktop OAuth orchestration in `app/oauth.go`, existing helper HTTP client.

---

### Task 1: Add authenticated OAuth credential handoff contract

**Files:**
- Modify `internal/automation/v1/account_mutations.go` and `errors.go` for request validation and stable failures.
- Modify `internal/automation/client/client.go` for the one-time request and secret-buffer clearing.
- Modify `internal/alwayson/catalog_api.go` and `sync.go` to mount and implement the account-scoped operation.

**Steps:**
1. Define a request containing provider, access/refresh tokens, expiry, granted scopes, stable provider identity, and optional custom-provider endpoints/client credentials.
2. Validate account existence, OAuth-compatible account type, token/provider consistency, and custom-provider completeness before storage.
3. Reserve idempotency using the device-keyed fingerprint from HOSTED-SERVER-036; store only the public acknowledgement.
4. Snapshot existing tokens and custom-provider data before writes; restore on a failed write and return a stable repair-required error if restore fails.
5. After successful persistence, refresh the helper's account/pool/scheduler state and return only accepted status and operation identity.

### Task 2: Route desktop OAuth completion and reauthorization through helper

**Files:**
- Modify `app/oauth.go` for new-account OAuth, custom-provider OAuth, and existing-account reauthorization.
- Modify `internal/automation/client/client.go` if operation-specific request types need adjustment.

**Steps:**
1. Replace direct account OAuth token/provider writes with the authenticated handoff when the helper is connected.
2. Keep the current local path only when the helper is unavailable; surface authorization and API failures without local fallback.
3. Preserve pending OAuth values after a failed handoff; clear them only after acknowledgement.
4. Propagate successful Microsoft token refreshes to linked shared-mailbox records inside the helper operation.
5. Confirm helper sync and SMTP consumers resolve refreshed credentials after acknowledgement and do not depend on the desktop process.

### Task 3: Document and verify the handoff

**Files:**
- Update `docs/API.md`, `docs/api/agent-v1.md`, `CHANGELOG.md`, and `TASKS.md`.

**Steps:**
1. Document request fields, capability, secret handling, retry behavior, and repair-required response.
2. Run `gofmt`, focused `go build` for the affected Go packages, and `git diff --check`; do not run tests unless requested.
3. Review every OAuth persistence call in `app/oauth.go` and confirm the service-connected path no longer writes mailbox OAuth tokens or custom provider secrets locally.
4. Record verification and remaining limitations in `TASKS.md`, commit the implementation, and push the task branch for coordinator integration.
