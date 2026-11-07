# Always-On Mail Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Run one always-on Postilaatikko service that keeps mailbox metadata current, applies local cross-account filters/moves, and delivers scheduled messages while the desktop UI is closed. The same service can run on localhost or on a user-operated Mac Studio/VPS.

**Architecture:** Make a shared Go service/core the single implementation of mailbox operations and background workflows. The desktop UI and any remote client call the same service API; localhost and user-hosted deployments differ by endpoint, transport security, and installation configuration, not duplicated business logic. Reuse existing account/sync, cross-account filter/move, and `internal/scheduledsend` code where their storage and lifecycle contracts fit. Start with one user's local service, then support that same service on a user-operated Mac Studio/VPS. Postilaatikko Cloud is explicitly deferred and may never ship. Keep mailbox bodies out of routine sync and retain periodic sync as recovery for missed provider notifications.

**Tech Stack:** Go; existing account, credential, Gmail API, IMAP sync, and scheduled-send packages; SQLite; Cloud Pub/Sub only if the selected deployment uses Gmail `users.watch` push delivery.

---

## Current state and design boundary

- `cmd/postilaatikko-helper` is an always-on scheduled-send endpoint. It reads an existing Postilaatikko account database and credentials from the host, runs the durable send worker, and can bind outside loopback only with TLS.
- `internal/automation/server.go` is a separate opt-in, loopback-only API. It is not a public or Pub/Sub callback endpoint.
- The desktop app owns mailbox sync today. Its scheduler syncs due inboxes; routine sync is metadata-focused. The app can remain local-only.
- The scheduled-send helper is trusted with sending credentials and decrypts queued MIME to send it. Extending that host to fetch mail makes it a trusted account-data processor too. Do not claim E2EE for data the host can decrypt or query.
- Gmail push uses Cloud Pub/Sub and is delivered to a backend. Watches expire and need renewal; polling must remain as recovery. A desktop loopback listener cannot receive Google callbacks.

## Product and deployment decision

The first deployment is the user's own service, normally on localhost and optionally on a Mac Studio or VPS. The operator controls the host, database, credentials, backups, TLS and access. Postilaatikko Cloud is a later possibility, not a dependency or target for this implementation.

The desktop must become a client of shared service operations instead of maintaining a second implementation of filtering, moving, sync, or scheduled delivery. Extract/reuse domain use cases incrementally from `app`; keep Wails bindings as a UI adapter and add a service API adapter over those same use cases. A local endpoint and a TLS-protected remote endpoint must call the same operation layer. Preserve the existing single-user trust model; do not add shared-cloud onboarding, multi-tenant identity, or SaaS operations in this scope.

The service owns background sync, automatic local filters, and delayed sends while the UI is closed. Quitting the desktop with Command-Q must not stop or restart the service. Install/supervise it independently (launchd on macOS) and give it a small headless runtime with no Wails/WebKit UI. The desktop may connect to or start the service when opened, but closing its windows must not terminate the service; stopping the service is a separate explicit service-management action.

In particular, new-message handling must evaluate the same persisted cross-account filter rules and use the existing safe move pipeline: confirm the destination copy before deleting the source. User-initiated move/filter edits must use the same use cases through the service API. Avoid having the desktop and service race as independent sync/mutation owners; define a clear service lifecycle and migration/DB ownership before switching desktop operations over.

## Milestones

### Current runnable slice

- `cmd/postilaatikko-helper serve --sync` supervises the Send Later worker and the existing bounded, header-first sync scheduler in a separate process on localhost. `install-agent --sync` opts the macOS LaunchAgent into that sync worker. `/healthz` is a no-store process check and returns no mailbox state.
- `internal/mailfilter.AutomaticRouter` is now independent of Wails and the desktop wires it to the existing app move path. `internal/mailmove.CopyBeforeCleanup` centralizes the core cross-account safety ordering and the app uses it.
- `internal/mailmove.FilterOperation` owns the shared automatic-move preconditions: destination must match the rule, source and destination accounts must differ, and the synced message must still be present in its original folder. The desktop supplies its undo/mutation-lease aware move adapter; the helper supplies its IMAP adapter, which confirms destination APPEND before source cleanup.
- The headless sync runtime routes newly persisted messages through the same rule router and shared preconditions. Automatic filter jobs are stored in SQLite; pending jobs replay after restart, while jobs interrupted after being claimed become `outcome_unknown` and are not retried automatically because APPEND acknowledgement can be ambiguous. Without UIDPLUS the adapter flags the copied source for provider cleanup instead of issuing a broad EXPUNGE. Orderly shutdown drains pending work with a 30-second ceiling.
- The desktop and helper coordinate background sync through an advisory process lock in the shared data directory. The desktop starts polling and IMAP IDLE only when it owns the lock, and direct desktop sync calls return an ownership error when the helper holds it. The desktop waits for the lock and resumes its scheduler if the helper exits while the app remains open. The helper keeps its Send Later endpoint available while waiting to acquire the lock, then starts sync after the desktop releases it.
- This remains a transition slice, not a full ownership cutover: the desktop UI/API handoff is not connected, and both processes still open the same local database and perform local operations directly. The lock coordinates sync lifecycles only; it does not establish one database-migration owner or make the desktop a service client. Uncertain filter jobs currently require database-level inspection; there is no user-facing recovery workflow yet.

### 1. Shared operation layer and service lifecycle

**Files:** `app/filter.go`, app move/sync use cases, `cmd/postilaatikko-helper/main.go`, new shared service package under `internal/alwayson/`, UI and service API adapters.

1. Identify the domain operations currently reachable only through `App` and factor service interfaces/use cases for sync, filter evaluation, safe moves, and scheduled delivery. Keep provider logic and safety invariants in one implementation.
2. Make the Wails app and the local/remote API adapters call the shared use cases. Do not copy filter matching or move behavior into a daemon or frontend.
3. Add an independent headless service entrypoint with configuration for account data, scheduled-send queue, sync policy, and enabled workers. Register it with the OS user-service manager so it starts independently of the UI.
4. Make the service the single owner of background sync and schema migrations; define how UI reads/writes are serialized through the API before moving persistent ownership.
5. Supervise scheduled-send and sync/filter workers under one context. Worker failures must be visible without silently killing unrelated work. Desktop shutdown must not cancel service workers.
6. On service shutdown, stop new sync work, wait for bounded in-flight sync and delivery, and preserve ambiguous-send recovery semantics.

### 2. Header-first background mailbox sync

**Files:** `internal/sync/`, the new service package, `cmd/postilaatikko-helper/` tests.

1. Reuse the existing `sync.Engine` and scheduler rather than copying provider-specific sync logic.
2. Sync only configured folders/accounts and message headers/flags on a bounded cadence. Do not fetch bodies or attachment content in the service's normal poll.
3. Prefer provider change signals where available: maintain IMAP IDLE as a hint; use Gmail Pub/Sub only after the service has a reachable authenticated receiver and renewable watch registration. Always retain a slow periodic recovery pass.
4. Coalesce notifications and requests per account/folder, enforce provider backoff and cancellation, and surface account-specific failures without blocking healthy accounts.
5. Test with fake IMAP/Gmail providers for added/removed messages, flag changes, disconnects, duplicate notifications, backoff and missed-notification recovery.

### 3. Fast desktop client and sent-message reconciliation

**Files:** existing client API/event contract, `internal/automation/` or a dedicated hosted-service API, app sync/send paths, generated Wails bindings only if needed.

1. Define the service API and per-device authentication for health, mailbox changes, paginated folder/conversation headers, user operations, body-on-demand reads, and change cursors. Do not expose the current loopback automation API by relaxing its host/origin checks.
2. Keep list/read APIs bounded and header-first. The desktop can render cached headers immediately, request the visible page and a small first-screen prefetch, then fetch message bodies explicitly. A client presentation cache may improve perceived speed, but the service remains authoritative for mailbox state.
3. Publish account-qualified change events or cursors so the client can update/invalidate only affected views instead of reloading whole folders. Keep localhost and TLS remote transports over the same API and operation layer.
4. After an in-app send is accepted, immediately add/refresh that message in the visible conversation and Sent view; reconcile later provider metadata idempotently so sync cannot duplicate it.
5. When a device reconnects, replay from a durable per-account cursor or trigger an incremental provider sync; missed push notifications must not lose updates.
6. Test duplicate/out-of-order events, reconnect, cursor expiry, concurrent sends and refresh, and account isolation.

### 4. Local and user-hosted deployment, operations and security review

**Files:** helper deployment docs/scripts, `docs/SEND_LATER.md`, `docs/API.md` or new hosted-service docs, `ROADMAP.md`, `TASKS.md`, privacy/terms documents if the operator is Postilaatikko.

1. Provide a least-privilege local service install/upgrade path (including launchd) and a documented user-operated Mac Studio/VPS deployment using the same binary and API.
2. Keep localhost access constrained and authenticated; require authenticated TLS for remote access, short/revocable per-device credentials, bounded requests/concurrency, and secrets excluded from logs.
3. Document backup/restore, key rotation, token revocation, queue retention, database migration, health checks and manual recovery of ambiguous sends.
4. Run Go race tests for stateful workers and provider fixtures, security/lint checks, then perform a separate source review before a live-account pilot.
5. Do not claim reliable scheduled delivery or real-time sync until restart, network loss, token expiry, duplicated/missed notifications and provider-accepted-but-unacknowledged sends are verified on the target deployment.

## Acceptance criteria

- The host performs bounded metadata sync while the UI is closed, and the desktop can detect/reconcile updates after reconnect.
- Quitting the desktop leaves the independently supervised service running; only an explicit service stop or OS logout/shutdown ends its workers.
- The desktop can display cached or paginated headers without waiting for full message bodies, and receives targeted change information rather than reloading entire folders for each update.
- Persisted cross-account filter rules continue to run while the UI is closed, using the shared filter and safe-move implementation; the confirmed destination copy precedes source deletion.
- Desktop and service adapters invoke one shared set of mail operations; no duplicate filter/move business logic is introduced.
- Routine sync fetches no message bodies or attachment content.
- In-app sent messages appear immediately after provider acceptance; later sync does not create duplicates.
- Scheduled delivery continues using durable queue states, exact-byte retries, and `outcome_unknown` handling. No ambiguous send is retried automatically.
- The local service is the default deployment. A user-operated remote Mac Studio/VPS uses the same service and operation layer; remote requests require authenticated TLS and revocable device access.
- A missed/dropped push notification is recovered by incremental polling.
- Postilaatikko Cloud remains out of scope. The deployment operator and the fact that the user-operated host can access tokens/message content are disclosed clearly.
