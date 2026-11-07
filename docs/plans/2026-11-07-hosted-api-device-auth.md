# Hosted API Device Authorization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Persist authenticated mailbox API client grants and provide a local helper CLI for creating, listing, and revoking those clients.

**Architecture:** Store client IDs, capability grants, revocation state, and SHA-256 bearer-token digests in the existing account database. Return the bearer token only from the create command, where the operator can copy it into a future API client. Keep the existing loopback server origin/host protections and capability checks unchanged; this change does not expose mailbox routes or create a remote listener.

**Tech Stack:** Go, SQLite through `internal/database`, the existing `internal/automation.TokenStore` contract, helper CLI.

---

### Task 1: Add the durable client-grant schema and store

**Files:**
- Modify: `internal/database/migrations.go`
- Create: `internal/automation/sqlite_token_store.go`
- Modify: `internal/automation/auth.go`

Persist a generated client ID, display name, token digest, capability list, creation timestamp, and revoked flag. Hash bearer tokens before database lookup or storage. Return raw tokens only in the `RegisterClient` result, and make list results omit them. Reject blank names and clients with no recognized capabilities. Use parameterized SQL and preserve the current `TokenStore` interface.

### Task 2: Add local helper management commands

**Files:**
- Modify: `cmd/postilaatikko-helper/main.go`
- Modify: `docs/API.md`
- Modify: `CHANGELOG.md`

Add `api-client create`, `api-client list`, and `api-client revoke` commands. They must open an already configured account database, run migrations, and close the database on every path. `create` accepts an explicit name and one or more known capabilities, prints the client ID and token once, and never stores or logs the raw token. `list` prints names, IDs, capabilities, creation time, and revocation state only. `revoke` requires a client ID and is idempotent only for an existing grant.

Document that a created credential is a bearer secret, is shown once, and is usable only by API endpoints that have been wired and exposed separately. Do not imply that this command enables remote access or a mobile pairing flow.

### Task 3: Record roadmap execution state and verify the helper build

**Files:**
- Modify: `TASKS.md`
- Review: `ROADMAP.md`

Record the schema, one-time token output, revocation behavior, scope boundary, and verification. Keep the roadmap unchanged unless implementation changes a durable product or security decision. Run `gofmt`, focused `go build` commands for `internal/automation` and `cmd/postilaatikko-helper`, and `git diff --check`; GitHub PR CI supplies the repository test and static-analysis run.
