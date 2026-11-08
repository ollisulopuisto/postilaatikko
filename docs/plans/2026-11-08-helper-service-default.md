# Helper Always-On Service Default Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Make the installed macOS helper agent own bounded mailbox sync and automatic filters by default, so the service continues after the desktop closes.

**Architecture:** Preserve `serve` as an explicit foreground command whose sync worker remains opt-in. `install-agent` will install the persistent local service with sync enabled unless the operator passes `--no-sync`; an explicit `--sync` remains accepted, while contradictory flags fail before writing the LaunchAgent. Both modes continue to use the shared sync-owner lock, so desktop and helper never run background sync concurrently.

**Tech Stack:** Go CLI, macOS LaunchAgent plist, helper sync runtime, Markdown operator documentation.

---

### Task 1: Define effective sync configuration for helper commands

**Files:**
- Modify: `cmd/postilaatikko-helper/main.go`

**Steps:**
1. Add a `--no-sync` flag and reject simultaneous `--sync` / `--no-sync` before opening service resources.
2. Keep `serve` sync disabled unless `--sync` is passed; make `install-agent` sync enabled unless `--no-sync` is passed.
3. Validate Gmail Pub/Sub flags against the effective sync value and pass that effective value to `serve` or `installAgent`.
4. Preserve the existing LaunchAgent plist argument behavior: include `--sync` only when the effective install configuration enables the worker.

### Task 2: Update operator guidance and release notes

**Files:**
- Modify: `docs/SEND_LATER.md`
- Modify: `docs/API.md`
- Modify: `CHANGELOG.md`

**Steps:**
1. Document that `install-agent` now enables bounded header sync and automatic filters by default; show `--no-sync` for Send Later-only installation.
2. Correct the stale warning that the desktop must be closed before `serve --sync`; explain the process lock handoff and that only one process owns background sync.
3. Update the API boundary summary to reflect integrated desktop conversation, search, thread/body, and mail-action routes while retaining explicit body fetch.
4. Add an Unreleased changelog entry describing the install default and opt-out.

### Task 3: Verify and prepare the task for integration

**Files:**
- Modify: `TASKS.md`

**Steps:**
1. Build the helper command and run `git diff --check`; do not run automated tests under the current session instruction.
2. Review default/opt-out flag flow and confirm no LaunchAgent file is written before invalid flag combinations are rejected.
3. Record verification and commit information in the task row, then commit and push the task branch for review.
