# Desktop Always-On Service Controls Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task.

**Goal:** Let users install, start, and stop the bundled macOS mailbox service from General Settings while keeping it independent of the desktop window lifecycle.

**Architecture:** Add explicit helper commands to inspect and control the per-user LaunchAgent. Stop unloads the service but retains its plist; uninstall remains available to remove it. Wails resolves and invokes only the helper binary bundled beside the desktop executable, passing the app's exact data and queue directories. Settings displays installed/running status and exposes an explicit action for the current state; non-macOS platforms receive no lifecycle controls.

**Tech Stack:** Go CLI, macOS LaunchAgent/launchctl, Wails bindings, Svelte settings component, localized JSON strings.

---

### Task 1: Add idempotent LaunchAgent lifecycle commands

**Files:**
- Modify: `cmd/postilaatikko-helper/main.go`
- Modify: `cmd/postilaatikko-helper/launchd_darwin.go`
- Modify: `cmd/postilaatikko-helper/launchd_other.go`

**Steps:**
1. Add `agent-status`, `start-agent`, and `stop-agent` helper commands that do not require loading a sending identity.
2. Report whether management is supported, whether the plist is installed, and whether `launchctl` reports the agent running.
3. Make start idempotent for an already running agent; make it bootstrap an installed plist and fail clearly if none exists.
4. Make stop idempotent and unload the agent without deleting its plist; make uninstall call stop and then remove the plist.
5. Keep non-macOS builds explicit: status reports unsupported and start/stop return a clear platform error.

### Task 2: Expose safe lifecycle controls through Wails

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `frontend/wailsjs/go/app/App.js`
- Modify: `frontend/wailsjs/go/app/App.d.ts`

**Steps:**
1. Resolve the helper executable beside the running macOS app with symlink resolution; do not invoke a shell or accept arbitrary executable paths.
2. Add Wails methods to query lifecycle state, install with explicit app data/queue paths, start an installed agent, and stop it while preserving its configuration.
3. Bound command execution, return actionable errors, and never include bearer credentials or mailbox content in arguments or output.
4. Keep the existing mailbox API grant and connection status independent from service process status.

### Task 3: Add localized General Settings controls

**Files:**
- Modify: `frontend/src/lib/components/settings/MailboxServiceAccess.svelte`
- Modify: `frontend/src/lib/i18n/locales/*.json`

**Steps:**
1. Display service-installed/running state and an explicit Install, Start, or Stop action.
2. Explain that stop affects the independent background service, while closing the desktop does not stop it.
3. Hide lifecycle controls when the platform does not support the bundled helper LaunchAgent.
4. Preserve the existing write-authorization flow and show lifecycle failures without replacing connection status.

### Task 4: Update service documentation and verify

**Files:**
- Modify: `docs/SEND_LATER.md`
- Modify: `docs/plans/2026-11-06-always-on-mail-service.md`
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`

**Steps:**
1. Update the current-state section to record integrated desktop API routing, default LaunchAgent sync, and the remaining shared-database/migration-owner boundary.
2. Document desktop install/start/stop and independent service lifetime.
3. Build the app and helper, parse updated locale JSON, and run `git diff --check`; do not run automated tests under the current session instruction.
4. Record verification and commit information in the task row, then commit and push the branch for coordinator integration.
