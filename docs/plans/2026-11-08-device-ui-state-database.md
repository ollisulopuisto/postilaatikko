# Device UI State Database Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move persisted desktop UI state out of the helper-owned `aerion.db` into a desktop-owned `device.db` while preserving existing state.

**Architecture:** Keep `aerion.db` service-owned and open under the existing schema-owner lock. Open a second, desktop-only SQLite file with a minimal `app_state` schema; copy legacy state rows with insert-if-absent semantics, never delete the source rows, and direct all subsequent app-state reads and writes to the desktop database. This is an incremental extraction and does not move settings, contacts, credentials, or mailbox stores.

**Tech Stack:** Go, `database/sql`, the existing SQLite `database.Open` wrapper, `internal/appstate`, platform data paths, Wails app lifecycle.

---

### Task 1: Add the desktop database path and app-state schema bootstrap

**Files:**
- Modify: `internal/platform/paths.go`
- Modify: `internal/appstate/store.go`
- Create: `internal/appstate/database.go`

**Steps:**
1. Add a stable `DeviceDatabasePath()` resolving to `<Data>/device.db`.
2. Add an app-state database initializer that creates only the `app_state` table and its required columns. It must not call the service database migration list.
3. Add an importer that reads all legacy `app_state` rows from a supplied service DB and inserts them into the device DB with `ON CONFLICT DO NOTHING` in a transaction. Missing legacy table means there is nothing to import; other read/write errors fail initialization.
4. Preserve the source rows. Repeated imports must be safe and must never replace newer device-local values.

### Task 2: Wire the second database into desktop lifecycle

**Files:**
- Modify: `app/app.go`

**Steps:**
1. Add an `App` field for the device DB and open it during preflight after paths are ensured and the mailbox DB is available.
2. Initialize the minimal schema and import old app-state rows before stores are constructed.
3. Construct `appStateStore` from the device DB. Keep the mailbox DB and schema lock unchanged for all other existing stores.
4. Close the device DB on startup failure and normal shutdown, preserving the existing mailbox DB cleanup and lock release behavior.
5. Inspect the path helper and changed call sites; do not add or run tests under the current session instruction.

### Task 3: Record the ownership boundary and verify

**Files:**
- Modify: `docs/plans/2026-11-06-always-on-mail-service.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `TASKS.md`

**Steps:**
1. Record `app_state` as desktop-owned and identify the legacy import as copy-only; keep the source table for rollback and other old clients.
2. Record the remaining shared service DB access as unfinished; this task does not complete server separation.
3. Run `gofmt` on changed Go files and `git diff --check`.
4. Run a focused build for `./app` and `./cmd/postilaatikko-helper`; do not run tests.
5. Update the task row with commit, verification and limitations, then commit the task branch.
