# Remote Compose and Send Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let the remote-only desktop compose, autosave, reply, and send through the mailbox helper without opening local mailbox or credential stores.

**Architecture:** Reuse the existing authenticated helper APIs and the detached-composer service proxies. Add remote-aware branches at the main desktop compose Wails boundary, where inline compose currently calls local draft/provider stores directly. Keep helper revisions and stable send receipt keys in the flow, use helper account/identity/reply APIs, and fail closed if an operation still requires a local-only crypto or attachment integration. Local-helper mode keeps its current implementation.

**Tech Stack:** Go, existing `internal/automation/client` v1 APIs, Wails app bindings, Svelte composer API.

---

### Task 1: Trace compose bindings and existing helper contracts

**Files:**
- Inspect: `app/draft.go`, `app/compose.go`, `app/account.go`, `app/mailbox_service.go`
- Inspect: `frontend/src/lib/composerApi.ts`, `frontend/src/ComposerApp.svelte`
- Inspect: `internal/automation/client/client.go`, helper draft/send handlers

1. Map inline and detached draft/send/reply call paths, including the send retry UI.
2. Identify available account-qualified draft revision, reply preparation, send idempotency and receipt APIs.
3. Confirm remote mode cannot use `draftOps`, `composeOps`, local account/credential stores or provider sync.

### Task 2: Add remote-aware draft and reply boundary behavior

**Files:**
- Modify: `app/draft.go`
- Modify: `app/compose.go`
- Test: focused `app` remote compose tests

1. Route remote draft read/list/update/delete through helper APIs and preserve helper revision checks.
2. Route remote reply preparation through the helper using account-qualified message references.
3. Ensure helper authorization, unavailable-server and stale-revision errors remain visible; never return a successful empty draft on failure.
4. Guard unsupported local crypto/attachment paths explicitly.

### Task 3: Route remote inline send through durable helper receipts

**Files:**
- Modify: `app/compose.go`
- Modify: `frontend/src/lib/composerApi.ts` if a stable request ID must cross retries
- Test: focused remote send retry and authorization cases

1. Prepare an immutable RFC822 payload and stable request ID before the first helper send attempt.
2. Reuse the same key/payload for retries and query helper receipts after ambiguous transport outcomes.
3. Require `mail.send` and `draft.write` where applicable; surface `outcome_unknown` without attempting a second send.
4. Do not run local Sent-folder sync in remote mode.

### Task 4: Verify and record

1. Run focused Go tests for the new remote compose routing and affected package builds.
2. Run `git diff --check`; run frontend checks only if dependencies are present.
3. Update `CHANGELOG.md`, this plan status, and the HOSTED-SERVER-052 row with verification/limitations.
4. Commit the isolated branch; coordinator integrates and pushes.

## Status

Remote inline draft read/write/delete/list, account-qualified reply preparation, and immediate send now use the existing helper APIs. The desktop retains one immutable prepared MIME request per send idempotency key while running and stores the opaque key in local storage for retry after a frontend reload. Remote mode does not initialize mailbox/provider/credential stores. Encrypted drafts, signing/encryption, and the local Undo Send pending queue remain unsupported until their keys and lifecycle are helper-owned. Focused app tests and affected Go builds pass; frontend static checks are pending dependency availability.
