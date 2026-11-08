# Account Service API Design Plan

**Goal:** Specify a safe ownership boundary for account setup, account configuration, and provider credentials before moving those writes out of the desktop.

**Architecture:** Audit current Wails account operations, OAuth callback/token persistence, credential encryption/keyring behavior, helper runtime dependencies, and detached composer. Define authenticated API operations and their capability, request/response, replay, rollback, and lifecycle contracts. The helper remains the only writer of account and provider credential state after cutover; secret material is accepted only by narrow mutation operations and is never returned by account/catalog reads.

**Tech Stack:** Go, `net/http`, account and credential stores, helper OAuth transport, desktop mailbox-service connector.

## Task 1: Claim and audit current behavior

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-account-service-api-design.md`
- Inspect: `app/account.go`, `app/oauth.go`, `app/oauth_creds.go`, `internal/account`, `internal/credentials`, helper transport/runtime composition, detached composer.

**Steps:**
1. Claim HOSTED-SERVER-033 before analysis.
2. Map each account, password, OAuth, custom-provider, shared-mailbox, enable/order, removal, and lifecycle operation to its current store and side effects.
3. Identify how to keep helper sync and SMTP transports current after account or credential changes.
4. Commit the claim and plan before continuing the audit.

## Task 2: Specify the service contract

**Files:**
- Modify: `docs/plans/2026-11-08-account-service-api-design.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`

**Steps:**
1. Specify account-management authorization separately from ordinary mailbox writes and describe desktop consent/revocation behavior.
2. Define account list/detail and create/update/remove/enable/order operations, credential handoff for IMAP/SMTP passwords and OAuth tokens, custom OAuth configuration, and shared mailbox dependencies.
3. Define secret non-disclosure, TLS requirements for remote endpoints, idempotent retries, cross-store rollback/recovery, and helper cache/sync lifecycle effects.
4. Split implementation into dependency-ordered tasks with explicit acceptance criteria. This task makes no runtime changes.

## Task 3: Review

1. Verify the design statements against current code and existing API capability/revocation behavior.
2. Run `git diff --check`; do not run tests because this task changes documentation only.
3. Update the task ledger with the design commit and remaining implementation sequence, then commit and push for coordinator integration.
