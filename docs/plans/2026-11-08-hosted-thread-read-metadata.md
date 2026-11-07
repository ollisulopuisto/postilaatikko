# Hosted Thread Read Metadata Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expand authenticated thread reads with the message headers and attachment summaries required for a remote desktop reader while keeping bodies and attachment bytes out of the response.

**Architecture:** Extend the shared `views.ThreadMessage` model with optional envelope, flag, security, and attachment-summary fields. Populate those fields from the headless runtime stores in `/v1/threads/read`, leaving `ContentState` as the only body-availability indicator. Mirror the additive contract in TypeScript and document the no-content invariant.

**Tech Stack:** Go, shared view contract, headless runtime stores, TypeScript contract mirror, Markdown API docs.

---

### Task 1: Extend the shared thread metadata contract

**Files:**
- Modify: `internal/views/model.go`
- Modify: `frontend/src/lib/views/types.ts`
- Modify: `frontend/src/lib/views/guards.ts`

Add optional header fields matching `message.Message` and a content-free attachment summary type. Preserve account-qualified references, folder identity, matched state, and body content state. Do not add body or attachment bytes.

### Task 2: Populate the authenticated API response

**Files:**
- Modify: `internal/alwayson/mail_read_api.go`
- Modify: `docs/API.md`

Map stored message headers and flags into `ThreadMessage`; fetch only attachment metadata from the attachment store and omit local paths/content. Keep the existing explicit body and attachment read endpoints as the only content paths.

### Task 3: Verify and record

Run `gofmt`, `git diff --check`, a focused Go build, and changed-package lint. Do not run local tests in this task environment. Update `TASKS.md` with the implementation and CI status before handoff.
