# Independent Mail Windows Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Open multiple usable mail-list windows with independent navigation, search, selection and layout.

**Architecture:** Reuse the authenticated detached-reader IPC and existing MessageList/ConversationViewer components. The parent remains the only owner of the database and sync engine; child windows maintain their own presentation state and receive mail events without accepting primary-window UI-state writes.

**Tech Stack:** Go, Wails v2, TypeScript, Svelte 5, existing Unix socket/Windows pipe IPC.

---

### Task 1: Protect window isolation and operation routing

**Files:** `app/mail_window_test.go`, `app/mail_window.go`, `app/detached_reader.go`, `app/ipc.go`; `frontend/tests/mailWindow.test.mjs`, `frontend/src/lib/windows/mailWindow.ts`.

1. Write failing table-driven Go tests covering the child mail-operation allowlist, forbidden administration/layout writes and argument-preserving mail RPC routing. Write Node tests for independent navigation, selection and width state.
2. Run `go test ./app -run 'TestMailWindow'` and `cd frontend && node --test tests/mailWindow.test.mjs`; confirm missing implementations fail.
3. Add mail-window configuration/mode and authenticated calls, validate startup scope, and forward bounded mail change notifications. Keep primary UI-state writes unavailable.
4. Run the focused tests again; expect pass.

### Task 2: Make independent mail windows usable

**Files:** `main.go`, `frontend/src/mailWindow.ts`, `frontend/src/MailWindowApp.svelte`, `frontend/mail-window.html`, `frontend/vite.config.ts`, `frontend/src/App.svelte`, `frontend/wailsjs/go/app/App.js`, `frontend/wailsjs/go/app/App.d.ts`, `frontend/src/lib/i18n/locales/en.json`, `frontend/src/lib/i18n/locales/fi.json`.

1. Add the detached mail entry point, account/folder selector, existing list/search and viewer, reply via durable detached compose, local width controls and refresh on focus.
2. Add a localized New mail window action in the main mail view and child toolbar. Preserve each window's folder and selection when another window navigates.
3. Run `npm run check`, targeted ESLint and `npm run build`. Exercise the component in a browser harness if available.

### Task 3: Verify and hand off

**Files:** `CHANGELOG.md`, `ROADMAP.md`, `TASKS.md`, `docs/coordination/handoff/2026-10-29-independent-mail-windows.md`.

1. Run focused race tests, `go test ./...`, `go build ./...` and `git diff --check`; record environmental/pre-existing failures precisely.
2. Record native WKWebView/real mail checks that still require the installed device. Mark P-013 Review, never Done before integration.
3. Inspect worktree status and commit only this task's files with `feat: add independent mail windows`.
