# Multi-Signal Move Filters Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Suggest a reviewable local filter after two separate user moves with matching headers to the same destination, let the user edit which criteria apply, and route future mail only after explicit confirmation.

**Architecture:** Record a bounded, local-only history of user-initiated move actions using message headers already in the local store. A backend suggester compares distinct actions scoped to source account and destination, derives stable sender/recipient/subject criteria, and emits a suggestion event. A dialog lets the user toggle and edit each criterion, select a valid local-rule destination, preview matching headers, then explicitly create a rule using the existing mailfilter service. Same-account Gmail-native creation remains deferred to P-060 and its incremental `gmail.settings.basic` consent.

**Tech Stack:** Go, SQLite migrations and stores, Wails bindings/runtime events, Svelte 5, existing `internal/mailfilter` rules, English/Finnish i18n.

---

### Task 1: Persist bounded user-move header history

**Files:**
- Modify: `internal/database/migrations.go` (new migration 64)
- Create: `internal/mailfilter/move_history.go`
- Modify: `app/app.go` (store initialization)
- Modify: `app/actions.go` (`MoveToFolderWithUndo` only)

Store one action ID per explicit user move and header-only samples for the moved messages. Keep sender, To/Cc, subject, source account/folder, destination and timestamp. Do not store body content. Retain a bounded recent history and exclude same-folder no-op moves.

### Task 2: Derive suggestions from independent move actions

**Files:**
- Modify: `internal/mailfilter/move_history.go`
- Modify: `app/filter.go`
- Modify: `app/mail_window.go` (bindings, if methods are needed from the frontend)

Require two distinct user action IDs to the same destination and source account. Derive only non-empty stable criteria shared across samples: exact normalized sender/recipient values and sufficiently distinctive common subject text. Keep criteria fields independent; later matching combines enabled fields with AND and alternatives in one field with OR. Suppress repeat offers after dismissal or successful creation using a stable suggestion key.

### Task 3: Support safe local destinations and matching

**Files:**
- Modify: `internal/mailfilter/model.go`
- Modify: `internal/mailfilter/match.go`
- Modify: `app/filter.go`

Allow local rules for same-account non-Gmail routing only when the source-folder scope is explicit and excludes the destination. Preserve cross-account confirmed-copy-before-delete behavior. Continue to defer same-account Gmail-native rules to P-060.

### Task 4: Review, preview, and create suggested rules

**Files:**
- Create: `frontend/src/lib/components/filter/SuggestedFilterDialog.svelte`
- Modify: `frontend/src/App.svelte` (single runtime-event listener and dialog owner)
- Modify: `app/filter.go` (header-only preview endpoint, if required)
- Modify: `app/mail_window.go` (Wails registration)

Show the account and destination. Provide separate enable/edit controls for sender, recipient and subject, with enabled fields combined using AND. Preview only matching local headers and never fetch message bodies. Create the local rule only when the user confirms; dismissing a suggestion suppresses that suggestion without changing mail.

### Task 5: Document the user-facing change

**Files:**
- Modify: `frontend/src/lib/i18n/locales/en.json`
- Modify: `frontend/src/lib/i18n/locales/fi.json`
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`

Describe the reviewed suggestion workflow in English/Finnish. Update P-057 with commit and verification details, and leave P-060 dependent and unclaimed until this path is integrated.

### Verification and limits

Use `git diff --check` and inspect the final branch diff. Automated tests and frontend checks are not run in this session. Live Gmail-native filter creation is out of scope until P-060; live provider delivery remains a separate release gate.
