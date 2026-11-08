# Hosted Filter Job Inspection Plan

**Goal:** Let authenticated mailbox-service clients inspect automatic filter moves whose provider outcome is uncertain, without retrying the move or changing mailbox data.

**Architecture:** Add a bounded, read-only `/v1/filter-jobs` API requiring `mail.read`. The helper reads durable `outcome_unknown` rows from `mailfilter.JobStore` and returns account-qualified source and destination identities, rule/job identifiers, timestamps, and the stored diagnostic. The API must not expose message content or permit mutation. A later task can design safe, explicit reconciliation after the operator can inspect these records.

**Tech Stack:** Go, `net/http`, SQLite `mailfilter.JobStore`, authenticated automation API.

## Task 1: Claim and map the persisted job contract

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-filter-job-inspection.md`
- Inspect: `internal/mailfilter/job_store.go`, schema migration 66, helper catalog API.

**Steps:**
1. Claim HOSTED-SERVER-031 before implementation.
2. Preserve the `outcome_unknown` no-automatic-retry invariant and expose only metadata needed to distinguish source, destination, rule, and failure.
3. Commit the claim and plan before code changes.

## Task 2: Add bounded authenticated inspection

**Files:**
- Modify: `internal/mailfilter/job_store.go`
- Create: `internal/automation/v1/filter_jobs.go`
- Modify: `internal/alwayson/sync.go`
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `internal/automation/client/client.go`

**Steps:**
1. Add stable, bounded pagination over uncertain jobs only, without changing job state.
2. Require `mail.read` and return account-qualified source and destination data, identifiers, error reason, and timestamps; exclude bodies and attachments.
3. Add a typed API client method.

## Task 3: Document and review

**Files:**
- Modify: `docs/API.md`, `CHANGELOG.md`, `TASKS.md`.

**Steps:**
1. Document the read-only route, pagination, and no-retry behavior.
2. Run `gofmt`, `git diff --check`, and focused Go builds. Do not run tests unless requested.
3. Record the commit and verification status, push for review, then integrate through the coordinator.

## Safety boundary

This slice only makes uncertainty visible. It does not retry, discard, complete, move, or delete any message/job. A recovery action requires a separate design that verifies destination and source state before permitting a retry or resolution.
