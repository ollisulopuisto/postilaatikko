# Composer Editing Ownership Implementation Plan

> **For Claude: REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.**

**Goal:** Restore reliable Cmd/Ctrl+V in all composer editing fields and route composer Undo to its local draft history while mailbox Undo remains helper-owned.

**Architecture:** Route the platform paste accelerator through Wails clipboard text and insert it at the active input/editor selection; notify detached composers over authenticated local IPC so only the focused child reads the clipboard. Keep the composer branching history in the front end and dispatch native Edit → Undo according to the focused window and editing surface; mailbox mutations continue through the authenticated helper's durable receipts.

**Tech Stack:** Go/Wails menu and runtime events; Svelte 5; TipTap/ProseMirror; existing composer draft-history API.

---

### Task 1: Trace and preserve native paste routing

**Files:**
- Inspect: `main.go` Edit menu Paste item.
- Inspect: `app/undo.go` native clipboard fallback.
- Inspect: `frontend/src/lib/nativePaste.ts` and both bootstrap entries.
- Inspect: `frontend/src/lib/components/composer/composerEditor.ts` paste plugin.

**Steps:**
1. Identify where Cmd/Ctrl+V is prevented or loses its focused editor target in the main and detached composer windows.
2. Route Cmd/Ctrl+V through the native menu accelerator, whose Wails clipboard API supplies plain text.
3. Insert into the active input selection or rich editor without losing focus; preserve selection replacement and editor change tracking.
4. Broadcast a paste request to detached composer windows; let only the focused child read clipboard text.
5. Record that native menu clipboard access currently exposes text only; verify the effect on rich HTML/image paste on macOS before claiming that behavior.

### Task 2: Route Undo by focused ownership

**Files:**
- Modify: `frontend/src/App.svelte` global Undo dispatch.
- Modify: `frontend/src/lib/components/composer/Composer.svelte` composer-local history command handling.
- Modify: `frontend/src/ComposerApp.svelte` detached-composer menu event handling if runtime routing requires it.
- Modify: `app/undo.go` or `app/detached_composer.go` only if a Wails event needs forwarding.

**Steps:**
1. Keep Cmd/Ctrl+Z and Cmd/Ctrl+Shift+Z inside a focused composer on its local branching history.
2. Route native Edit → Undo to local composer history when focus belongs to the composer.
3. Route Edit → Undo and global keyboard Undo to helper mailbox history when focus is outside a composer; ignore the main window's menu event while a detached composer owns focus.
4. Preserve operation-specific toast Undo and ensure one menu action cannot invoke both histories.

### Task 3: Record behavior and verify

**Files:**
- Update: `ROADMAP.md`, `TASKS.md`, and `CHANGELOG.md`.

**Steps:**
1. Document the ownership boundary and note any platform behavior that remains device verification.
2. Run `git diff --check` and the focused Go build if backend files change.
3. Run available frontend static checks only if dependencies are present; otherwise record that they were unavailable.
4. Commit the focused task branch and update its task row with verification and limitations.
