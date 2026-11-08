# Hosted Server Gmail Credential Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let helper-owned mailbox operations use the Gmail API with OAuth credentials already configured locally, without transmitting credentials through the service API.

**Architecture:** Expose a scoped Gmail token-source method from the helper's local scheduled-send credential transport, then inject it into the helper sync engine before service startup. Reuse stored scope metadata and existing refresh logic. Preserve IMAP support and report reauthorization or capability failures through the existing engine behavior.

**Tech Stack:** Go, helper-local OAuth credential store, Gmail REST client, sync engine.

---

### Task 1: Claim and verify the credential boundary

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-hosted-server-gmail-credentials.md`
- Inspect: `internal/scheduledsend/smtp_transport.go`, `internal/sync/gmail_native.go`, `cmd/postilaatikko-helper/main.go`

**Steps:**
1. Claim HOSTED-SERVER-028 before implementation.
2. Confirm the helper already reads OAuth tokens from its local credential store and refreshes them through its OAuth manager.
3. Keep tokens, refresh tokens, and client secrets local to the helper; do not add API fields or logging for them.
4. Commit the claim and plan before code changes.

### Task 2: Supply Gmail API tokens to the helper engine

**Files:**
- Modify: `internal/scheduledsend/smtp_transport.go`
- Modify: `internal/alwayson/sync.go`
- Modify: `cmd/postilaatikko-helper/main.go`

**Steps:**
1. Add an exported token-source method that validates Gmail OAuth account/provider and the persisted search scope.
2. Reuse the transport's existing expiry/refresh path and return only the access token to the sync engine.
3. Add a runtime configuration method that installs the token source before startup.
4. Wire the helper `serve` command to pass its local transport token source to the runtime.
5. Preserve `gmail.ErrNotCapable` for non-Gmail accounts and reauthorization errors for unsupported grants.

### Task 3: Document and hand off

**Files:**
- Modify: `docs/plans/2026-11-06-always-on-mail-service.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `TASKS.md`

**Steps:**
1. Document that the helper uses its local credential store for Gmail API tokens.
2. Run `gofmt`, `git diff --check`, and builds for affected Go packages. Automated tests are excluded for this session.
3. Record verification and live-Gmail limitations, commit, and push the task branch.
