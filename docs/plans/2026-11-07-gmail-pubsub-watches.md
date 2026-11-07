# Gmail Pub/Sub Watches Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let the localhost always-on helper consume Gmail mailbox change notifications from an operator-configured Cloud Pub/Sub pull subscription, renew Gmail watches, and retain periodic sync as recovery.

**Architecture:** Reuse the existing Gmail history cursor and `SyncGmailAccount` path as the only mailbox change applier. Add a Gmail watch API method and an opt-in helper worker that starts only when both a topic and pull subscription are configured; it uses Application Default Credentials to pull notifications, maps `emailAddress` to an existing Gmail account, runs the incremental sync, and acknowledges only after success. Watch renewal runs at startup and daily, while bounded scheduled sync continues unchanged to recover dropped notifications and Pub/Sub outages.

**Tech Stack:** Go, existing Gmail REST client and sync engine, Google Cloud Pub/Sub Go client with ADC, existing helper/LaunchAgent configuration, SQLite Gmail history cursors.

---

### Task 1: Claim and record the Pub/Sub slice

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-07-gmail-pubsub-watches.md`

**Steps:**
1. Record HOSTED-SERVER-011 as In progress on its isolated branch/worktree based on latest main.
2. Keep the feature opt-in. The operator provisions a Pub/Sub topic and pull subscription and grants Gmail publisher access; the helper does not create cloud resources or require an inbound listener.

### Task 2: Add Gmail watch registration

**Files:**
- Modify: `internal/gmail/native.go`

**Steps:**
1. Add request/response structs and `Client.Watch(ctx, topic)` for `POST /watch`; reject empty topics and incomplete successful responses.
2. Parse Gmail's millisecond expiration into `time.Time`, retaining the returned history ID for diagnostics only; the persisted sync history cursor remains authoritative.
3. Verify the method compiles with the existing Gmail client API surface; CI runs the repository suite.

### Task 3: Implement the opt-in Pub/Sub receiver and watch renewer

**Files:**
- Create: `internal/alwayson/gmail_push.go`
- Modify: `internal/alwayson/sync.go`
- Modify: `go.mod` and `go.sum` if the official Pub/Sub client is not already available

**Steps:**
1. Define a narrow injectable Pub/Sub receiver interface so notification processing and ack/nack behavior can be tested without cloud access.
2. Start a watch for every configured Gmail OAuth account at worker startup, renew daily, and retry transient setup/renewal failures with bounded backoff; never stop the existing periodic scheduler because Pub/Sub setup failed.
3. Pull notifications using ADC. Treat the payload only as a sync hint: require a valid email address mapped to exactly one configured Gmail account, then call `SyncGmailAccount` so existing cursor/page handling, deduplication, header-only sync, and filter callbacks remain authoritative.
4. Ack only after successful incremental sync; nack or allow lease expiry on error. Bound concurrency per account and across the subscription; duplicate notifications must coalesce or serialize with the existing Gmail account lock.
5. Stop pulls and renewal cleanly with the runtime context. Log account-level setup/sync failures without exposing tokens or notification contents.

### Task 4: Configure helper and LaunchAgent

**Files:**
- Modify: `cmd/postilaatikko-helper/main.go`
- Modify: `cmd/postilaatikko-helper/launchd_darwin.go`
- Modify: `internal/alwayson/sync.go`

**Steps:**
1. Add explicit `--gmail-pubsub-project`, `--gmail-pubsub-topic`, and `--gmail-pubsub-subscription` flags. Require all three together and require `--sync`; empty configuration keeps the current behavior. Support an optional `--gmail-pubsub-credentials` path for a protected service-account credentials file, otherwise use ADC.
2. Pass the configuration to the runtime worker and persist the configured flags into LaunchAgent arguments from `install-agent`.
3. Ensure subscription client creation/configuration errors are surfaced while the periodic sync remains running where possible; the helper's existing lifecycle/shutdown still owns the worker.

### Task 5: Document, verify, and publish

**Files:**
- Modify: `docs/SEND_LATER.md`
- Modify: `docs/API.md`
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`

**Steps:**
1. Document Cloud project/topic/subscription setup, Gmail publisher IAM grant, local ADC setup, opt-in serve/install-agent flags, required Gmail/Cloud permissions, daily renewal, and periodic recovery behavior.
2. Run `gofmt`, `git diff --check`, and focused Go builds. Do not add or run tests locally per the active task instructions; rely on GitHub CI for the full suite.
3. Commit the implementation and review-status ledger update, push this task branch, open a PR against `main`, and leave it unmerged.

### Safety and reliability invariants

- No public inbound listener is added for localhost; the helper initiates Pub/Sub pull requests.
- Notification payload fields never advance or overwrite the durable Gmail sync cursor.
- A notification is acknowledged only after existing incremental sync succeeds; failures remain eligible for retry.
- The existing periodic scheduler stays enabled and is the recovery path for dropped notifications, watch expiry, Cloud outages, and service downtime.
- Pub/Sub setup is optional; no credentials, email addresses, or raw notification payloads are written to logs.
