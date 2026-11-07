# Headless Mailbox Catalog API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Give the headless service mailbox-backed, capability-checked v1 endpoints for account, folder, and saved-view metadata.

**Architecture:** Add a small catalog operation interface to the existing v1 API layer and implement it with the headless runtime's SQLite stores. Expose a reusable handler from `internal/alwayson` so a later helper-listener change can mount these routes under the existing authenticated `automation.Server`; do not add a second auth path, expose remote access, or retrieve message bodies.

**Tech Stack:** Go, `net/http`, `internal/automation/v1`, SQLite account/folder/virtual-folder stores.

---

### Task 1: Split catalog reads from the complete read API interface

**Files:**
- Modify: `internal/automation/v1/read.go`

Define `CatalogOperations` for account, folder, and virtual-folder listing. Keep `ReadOperations` as the complete read contract by embedding `CatalogOperations`. Add a route-mounting function for only the three catalog paths that reuses the existing response envelopes and `mail.read` capability checks. Existing full `ReadHandler` behavior stays wired through the same catalog interface.

### Task 2: Provide the runtime-backed handler

**Files:**
- Create: `internal/alwayson/catalog_api.go`

Build the catalog operations adapter over `account.Store`, `folder.Store`, and `virtualfolder.Store` using the runtime's existing database. Export a constructor returning an `http.Handler`; require the outer authenticated automation server to supply the request grant and use `automation.RequireGrantCapability` for each catalog route.

### Task 3: Update API documentation and verify

**Files:**
- Modify: `docs/API.md`
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`

Document that the headless catalog handler is ready for the helper's authenticated local server mount, and that the helper listener integration is a follow-up. Run `gofmt`, focused builds for `internal/alwayson` and `cmd/postilaatikko-helper`, and `git diff --check`; GitHub PR CI supplies repository test/static analysis checks.
