# Versioned Agent API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Define a stable, authenticated local API contract that lets external agents read mail safely and perform explicitly authorized draft, mailbox, send, and scheduling operations.

**Architecture:** UX-019 defines transport-independent Go request/response types and a human-readable v1 contract. UX-020 separately implements an opt-in loopback HTTP listener and caller grants; UX-021 separately binds read operations after UX-003 and UX-007 are integrated. CLI and MCP remain thin clients of the same authenticated contract. The contract reuses account-qualified references and completeness semantics from UX-002 and never treats message text as instructions.

**Tech Stack:** Go standard library types, existing `internal/views` contract, Markdown JSON examples.

---

### Task 1: Claim and reconcile existing API boundaries

**Files:** `TASKS.md`; inspect `internal/automation/v1/reply_later.go`, `internal/views/model.go`, and `docs/api/view-contract.md`.

Record UX-019 ownership before editing. Treat `ReplyLaterAdapter` as an existing in-process adapter, not a network endpoint. Reuse `views.MessageRef`, `ThreadRef`, `Scope`, `Cursor` and completeness definitions; do not redefine threading, query semantics, or local authorization behavior in UX-019.

### Task 2: Specify the v1 wire contract

**Files:** Create `docs/api/agent-v1.md`.

Document versioning, opt-in local access, bearer-header authentication, per-client capabilities, account-qualified stable IDs, opaque scope-bound cursors, bounded result/body/attachment sizes, response completeness, typed errors, request idempotency, and revision conflict behavior. Include examples for cached, explicitly fetched, partial, permission-denied and provider-error results. State that mail bodies, attachments, display names and headers are untrusted data; clients must not execute them or interpret them as API instructions. Keep credentials out of URLs, logs, telemetry and error bodies.

### Task 3: Add transport-independent shared types

**Files:** Create `internal/automation/v1/types.go` and `internal/automation/v1/errors.go`.

Represent capability names and validation, API version, bounded pagination, account-qualified references, structured failures and completeness without embedding HTTP concepts or secrets. Define forward-compatible JSON shapes that reference the shared view model. Add comments to exported Go declarations. Keep API errors safe for clients and preserve a separate internal cause for logging.

### Task 4: Review consistency and handoff

**Files:** `docs/api/agent-v1.md`; `internal/automation/v1/types.go`; `internal/automation/v1/errors.go`; `TASKS.md`.

Check that JSON examples agree with Go field names and UX-002 semantics. Mark UX-019 Review with commit and verification notes when ready. Keep UX-020 and UX-021 unclaimed until the predecessor is integrated and UX-021 dependencies UX-003/UX-007 are integrated. Do not claim an HTTP listener or usable external endpoint in this design slice.

**Verification:** Run focused Go contract tests and formatting checks when the coordinator requests verification; this coding session will not run tests unless requested. Perform a separate security/design review before integrating the contract.
