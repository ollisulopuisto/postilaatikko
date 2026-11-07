# Desktop Thread Open Service Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Route desktop thread metadata reads through the authenticated mailbox service when connected, while preserving the existing Wails conversation model and body-on-demand behavior.

**Architecture:** Use the account-qualified `/v1/threads/read` endpoint with the selected real folder scope. Convert the richer `ThreadView` header contract into the existing message/conversation models, overlay only bodies already cached in the desktop store, and preserve body availability as not fetched when content is absent. Local provider reads remain the unavailable-helper fallback; authorization and API failures are surfaced.

**Tech Stack:** Go Wails adapter, authenticated automation client, shared views contract, message store.

---

### Task 1: Add a typed connector method

**Files:**
- Modify: `app/mailbox_service.go`

Expose `ReadThread` through the existing authenticated connector with a caller context.

### Task 2: Route and adapt `GetConversation`

**Files:**
- Modify: `app/message.go`
- Modify: `app/mailbox_service.go` if helper status behavior needs a small adapter

Resolve account and real-folder scope, call the service when connected, map thread metadata and messages into `message.Conversation`, and reuse local bodies only when already cached. Never treat a missing local body as fetched. Keep explicit service-availability fallback and propagate authorization/capability/API errors.

### Task 3: Verify and record

Run `gofmt`, `git diff --check`, focused Go build, and changed-package lint. Do not run local tests in this task environment. Update `TASKS.md` with implementation and CI status before handoff.
