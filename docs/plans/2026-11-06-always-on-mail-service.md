# Always-On Mail Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Provide an optional always-on Postilaatikko service that keeps mailbox metadata current and delivers scheduled messages while the desktop UI is closed.

**Architecture:** Treat mailbox sync and scheduled delivery as workers owned by one long-lived service, reusing the existing account/sync and `internal/scheduledsend` code where their storage and lifecycle contracts fit. Start with a single-user deployment; do not assume a shared SaaS service. Keep mailbox bodies out of routine sync, make the service's account-credential and message-content trust boundary explicit, and retain periodic sync as recovery for missed provider notifications.

**Tech Stack:** Go; existing account, credential, Gmail API, IMAP sync, and scheduled-send packages; SQLite; Cloud Pub/Sub only if the selected deployment uses Gmail `users.watch` push delivery.

---

## Current state and design boundary

- `cmd/postilaatikko-helper` is an always-on scheduled-send endpoint. It reads an existing Postilaatikko account database and credentials from the host, runs the durable send worker, and can bind outside loopback only with TLS.
- `internal/automation/server.go` is a separate opt-in, loopback-only API. It is not a public or Pub/Sub callback endpoint.
- The desktop app owns mailbox sync today. Its scheduler syncs due inboxes; routine sync is metadata-focused. The app can remain local-only.
- The scheduled-send helper is trusted with sending credentials and decrypts queued MIME to send it. Extending that host to fetch mail makes it a trusted account-data processor too. Do not claim E2EE for data the host can decrypt or query.
- Gmail push uses Cloud Pub/Sub and is delivered to a backend. Watches expire and need renewal; polling must remain as recovery. A desktop loopback listener cannot receive Google callbacks.

## Deployment decision required before remote runtime work

1. **User-operated host:** one user's Mac Studio or VPS stores that user's account database, OAuth refresh tokens, and scheduled-send keys. Postilaatikko supplies the software; the operator controls host, backups, TLS and access. This best matches the existing helper's trust model.
2. **Postilaatikko-operated service:** a shared, multi-user deployment stores or can use customer OAuth credentials and processes mail data. This requires explicit account isolation, token/key management, user authentication, abuse controls, privacy/retention terms, incident response and operating-cost decisions. It is a separate product trust mode.

Do not implement public account onboarding or remote mailbox reads until the operator model is selected. A local prototype can exercise worker lifecycle and provider sync against a test account without making a production endpoint public.

## Milestones

### 1. Service lifecycle and shared storage

**Files:** `cmd/postilaatikko-helper/main.go`, new service package under `internal/alwayson/`, tests for lifecycle and cancellation.

1. Define a service configuration that names the account data directory, queue directory, sync policy, and enabled workers. Reject ambiguous or unsafe directory reuse.
2. Open account, credential, folder, message, and attachment stores once; avoid two independent processes racing schema migration or holding incompatible DB lifetimes.
3. Supervise the scheduled-send worker and sync worker under one context. One worker's failure must be visible and must not silently kill the other.
4. On shutdown, stop new sync work, wait for bounded in-flight sync and delivery work, and preserve ambiguous-send recovery semantics.
5. Add lifecycle tests for startup failures, cancellation, worker failure, and clean restart; run focused package tests and `go vet`.

### 2. Header-first background mailbox sync

**Files:** `internal/sync/`, the new service package, `cmd/postilaatikko-helper/` tests.

1. Reuse the existing `sync.Engine` and scheduler rather than copying provider-specific sync logic.
2. Sync only configured folders/accounts and message headers/flags on a bounded cadence. Do not fetch bodies or attachment content in the service's normal poll.
3. Prefer provider change signals where available: maintain IMAP IDLE as a hint; use Gmail Pub/Sub only after the service has a reachable authenticated receiver and renewable watch registration. Always retain a slow periodic recovery pass.
4. Coalesce notifications and requests per account/folder, enforce provider backoff and cancellation, and surface account-specific failures without blocking healthy accounts.
5. Test with fake IMAP/Gmail providers for added/removed messages, flag changes, disconnects, duplicate notifications, backoff and missed-notification recovery.

### 3. Desktop visibility and sent-message reconciliation

**Files:** existing client API/event contract, `internal/automation/` or a dedicated hosted-service API, app sync/send paths, generated Wails bindings only if needed.

1. Define a per-device authenticated channel for change cursors and service health. Do not expose the current loopback automation API by relaxing its host/origin checks.
2. Return only bounded account-qualified change metadata; readers still fetch message bodies explicitly through the selected trust path.
3. After an in-app send is accepted, immediately add/refresh that message in the visible conversation and Sent view; reconcile later provider metadata idempotently so sync cannot duplicate it.
4. When a device reconnects, replay from a durable per-account cursor or trigger an incremental provider sync; missed push notifications must not lose updates.
5. Test duplicate/out-of-order events, reconnect, cursor expiry, concurrent sends and refresh, and account isolation.

### 4. Deployment, operations and security review

**Files:** helper deployment docs/scripts, `docs/SEND_LATER.md`, `docs/API.md` or new hosted-service docs, `ROADMAP.md`, `TASKS.md`, privacy/terms documents if the operator is Postilaatikko.

1. Provide a least-privilege service install and upgrade path for the chosen host OS; retain current launchd behavior where applicable.
2. Require authenticated TLS for remote access, short/revocable per-device credentials, bounded requests/concurrency, and secrets excluded from logs.
3. Document backup/restore, key rotation, token revocation, queue retention, database migration, health checks and manual recovery of ambiguous sends.
4. Run Go race tests for stateful workers and provider fixtures, security/lint checks, then perform a separate source review before a live-account pilot.
5. Do not claim reliable scheduled delivery or real-time sync until restart, network loss, token expiry, duplicated/missed notifications and provider-accepted-but-unacknowledged sends are verified on the target deployment.

## Acceptance criteria

- The host performs bounded metadata sync while the UI is closed, and the desktop can detect/reconcile updates after reconnect.
- Routine sync fetches no message bodies or attachment content.
- In-app sent messages appear immediately after provider acceptance; later sync does not create duplicates.
- Scheduled delivery continues using durable queue states, exact-byte retries, and `outcome_unknown` handling. No ambiguous send is retried automatically.
- Account and device access are isolated and revocable; remote requests require authenticated TLS.
- A missed/dropped push notification is recovered by incremental polling.
- The deployment operator and the fact that the host can access tokens/message content are disclosed clearly.
