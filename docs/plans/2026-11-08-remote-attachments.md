# Remote Attachment Reads Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let a remote-only desktop list message attachments and fetch bytes only when rendering inline content or opening/saving a file.

**Architecture:** Reuse the helper's authenticated attachment list/read routes and account-qualified references. Add client methods for bounded metadata and raw bytes, then add explicit remote App methods so Wails/UI paths do not rely on local attachment/message stores. Retain the existing local path and the attachment downloader's safe filename/path handling.

**Tech Stack:** Go, `internal/automation/client`, existing v1 attachment routes, Wails App bindings, Svelte message reader.

---

### Task 1: Add authenticated client operations

**Files:**
- Modify: `internal/automation/client/client.go`
- Create: `internal/automation/client/attachments_test.go`
- Reuse: `internal/automation/v1/read.go` request/response contracts

1. Add metadata listing with `{accountId, messageId}` and decode only the summary fields already exposed by the helper.
2. Add byte reads with `{accountId, messageId, attachmentId}`; preserve raw bytes and content type.
3. Test request paths, authorization, metadata decoding, bytes/content type, and server errors.

### Task 2: Route remote attachment metadata and inline reads

**Files:**
- Modify: `app/attachment.go`
- Modify: `app/mailbox_service.go` only if a small adapter is needed
- Modify: `app/remote_workspace_test.go`

1. Add account-qualified App methods for listing attachments and loading inline CID data.
2. In remote mode, request inline bytes only for inline attachment metadata; bound concurrent reads and encode content as data URLs.
3. Propagate access, capability, and network failures instead of returning empty attachment sets.

### Task 3: Route open/save flows and UI account scope

**Files:**
- Modify: `app/attachment.go`
- Modify: `frontend/src/lib/components/viewer/AttachmentList.svelte`
- Modify: `frontend/src/lib/components/viewer/EmailBody.svelte`
- Modify: `frontend/src/lib/components/viewer/ConversationViewer.svelte`
- Modify: `frontend/wailsjs/go/app/App.js`

1. Thread the owning account ID from each message into attachment list and inline-body requests.
2. Add account-qualified remote open/save methods that fetch bytes explicitly and write them through existing safe download/path handling.
3. Keep local-helper methods and encrypted-message attachment paths unchanged.

### Task 4: Verify and record

1. Run focused client/app attachment tests and affected Go builds.
2. Run `git diff --check`; use frontend checks only if dependencies are installed.
3. Record verification and limitations in this plan and HOSTED-SERVER-053, commit the isolated branch, and integrate through the coordinator.

## Status

The helper API now verifies that the message belongs to the supplied account before listing metadata or returning attachment bytes. The desktop client lists metadata and explicitly reads bytes; remote UI uses account-qualified list, inline, open, save, and save-all methods. Inline reads are limited to 32 attachments, 16 MiB expected total, and four concurrent requests. Focused client/API/app tests, affected Go builds, and `git diff --check` pass. Frontend static checks remain unavailable because dependencies are absent; native save dialogs and provider attachment retrieval remain unverified on-device.
