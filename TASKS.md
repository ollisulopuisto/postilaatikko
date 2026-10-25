# Task Board

`ROADMAP.md` is the source of truth for product goals and priority. This file tracks work that can be claimed and delivered independently. Follow the repo-level [agent instructions](AGENTS.md).

## Coordination rules

1. Before editing code, claim one task below by setting its status to `In progress`, owner/model, branch, and worktree. Do not claim a task already in progress.
2. Use one branch and preferably one worktree per task. Start from the current `main`; do not share a working tree with another model.
3. Keep each task to one reviewable change. If tasks touch the same files, work on them sequentially or explicitly coordinate a single owner; do not edit those files concurrently.
4. Follow the task's acceptance criteria. Update the task status and this file when work is ready for review; do not mark it done until the change is integrated.
5. Update `ROADMAP.md` when product scope, priority, or durable decisions change. Do not rewrite its priorities just to claim a task.
6. A task may be split into smaller tasks when investigation reveals separate backend, UI, and migration work. Record dependencies before another model claims a dependent task.

## Status and effort

- **Easy:** contained UI, copy, localization, or isolated behavior change.
- **Medium:** feature spanning several components or backend/frontend contracts.
- **Large:** provider integrations, durable scheduling, broad architecture, or platform services.
- **Unclaimed / In progress / Review / Done:** task status. `Done` means integrated, not merely implemented in a separate branch.

## In progress in the current worktree

These tasks started in the primary checkout before `AGENTS.md` established worktree isolation. Do not join that checkout; the coordinator owns these changes.

| ID | Effort | Task | Owner / branch | Status |
| --- | --- | --- | --- | --- |
| P-001 | Easy | Add Finnish core and extension translations, locale registration, and key/placeholder parity. | Coordinator / `main` primary checkout | Done (v0.3.12) |
| P-002 | Easy | Add macOS Preferences shortcut and title-bar double-click maximize; show Postilaatikko in visible window chrome. | Coordinator / `main` primary checkout | Done (v0.3.12) |
| P-003 | Easy | Prefer account display names under Inbox and in account rows, falling back to email. | Coordinator / `main` primary checkout | Done (v0.3.12) |
| P-004 | Easy | Keep Edit Account dialog size stable across tabs and anchor the footer. | Coordinator / `main` primary checkout | Done (v0.3.12) |
| P-005 | Medium | Make routine sync header-only; fetch message bodies and attachment metadata when a message is opened. | Coordinator / `main` primary checkout | Done (v0.3.12) |

## In progress in separate worktrees

| ID | Effort | Task | Owner / branch / worktree | Status |
| --- | --- | --- | --- | --- |
| P-011 | Large | Critical E2EE Send Later: durable queue, pair clients with the Mac Studio helper, encrypt queued messages to the helper public key, and keep the private key and sending credentials on the helper. Relay stores ciphertext only; helper decrypts at delivery time. Include retry/recovery, cancellation, status, and missed due-time handling. Hosted decryption/sending is out of scope. | GPT-6.1-Sol (medium) / `agent/p011-e2ee-scheduled-send` / `/private/tmp/postilaatikko-p011` | In progress; checkpoint `7be5fe3` |
| P-014 | Medium | Add local cross-account filters for routing incoming messages. | opencode (Qwen3.8-27B) / `feat/p014-cross-account-filters` / `../postilaatikko-p014` | Done (v0.3.13) |
| P-012 | Easy | ICS event handoff (Easy): create an ICS event from structured date/time details and open it for review/import. | OpenCode / `agent/p012-ics-event-handoff` / worktree `../postilaatikko-p012` | Done (v0.3.13) |
| P-010 | Large | Gmail server-side search through the Gmail API with IMAP fallback (`internal/gmail`, `Engine.GmailSearch`, `App.searchFolderOnServer`), granted scopes persisted. See section 8 of `docs/plans/2026-10-24-gmail-api-contract.md`. Verified: `go test -race` for gmail, sync, oauth2; `golangci-lint` for those packages; `app` type-checks with `CGO_ENABLED=0` apart from two pre-existing cgo-only errors. Not verified: against real Gmail (the Gmail API may not be enabled in the OAuth client project), in the running app. | Claude (Sonnet 5.5) / `agent/P-010-gmail-api` / `../postilaatikko-p010` | Done (v0.3.13); commit `0243a94` |
| P-019 | Medium | Gmail-native mark-as-spam and not-spam through the Gmail API for Inbox <-> Spam moves (`Client.SetSpam`, `App.gmailSpamMove`, label-change branch in `moveMessagesToIMAP`), IMAP COPY+DELETE as fallback. Stacked on P-010 (PR #9). See section 9 of `docs/plans/2026-10-24-gmail-api-contract.md`. Verified: `go test -race` for gmail and sync; `app` type-checks with `CGO_ENABLED=0` apart from two pre-existing cgo-only errors. Not verified: against real Gmail, undo and UID reconciliation in the running app, classifier training behavior. | Claude (Sonnet 5.5) / `agent/P-019-gmail-spam` / `../postilaatikko-p019` | Done (v0.3.13); commit `e84e0bb` |
| P-018 | Easy | Reusable quick-reply snippets in the composer: `QuickReplies.svelte` (footer dropdown + manage dialog), local storage in the settings table, Go store tests. Verified: `go test ./internal/settings`, `svelte-check` (no new errors), `vite build`. Not verified: `app` package compile (GTK headers unavailable in this environment), Wails bindings were hand-edited to match generator output, behavior in the running app. | Claude (Sonnet 5.5) / `agent/P-018-quick-replies` / `../postilaatikko-p018` | Done (v0.3.13); commit `7e0f935` |
| P-025 | Easy | Audit the keyboard shortcut reference against current behavior. Documentation only: added `Ctrl+Z` undo, sync toggle/cancel behavior, macOS `⌘` equivalence, focus-mode and Escape details; corrected the composer-blocking note (global Mail shortcuts do not fire while the composer is open). Verified against `App.svelte`, `lib/keyboard/shortcuts.ts`, calendar/contacts shortcut files and `Composer.svelte`; not exercised in the running app. | Claude (Sonnet 5.5) / `agent/P-025-shortcut-audit` / `../postilaatikko-p025` | Done (v0.3.13); commit `5440041` |
| P-023 | Easy | Accessible names for the icon-only Contacts list controls (search clear/toggle, sort, add): `aria-label` reuses the existing localized `title` strings. Salvaged from closed Jules PR #2. Verified: `svelte-check`, `eslint`, `vite build`; not exercised in the running app. | Claude (Sonnet 5.5) / `agent/P-023-024-a11y` / `../postilaatikko-a11y` | Done (v0.3.13); commit `fe4a7bf` |
| P-024 | Easy | Localized the hard-coded labels in `kit/DetailOverlay.svelte` and `kit/RichTextEditor.svelte` using existing `common.*`, `viewer.exitFocus` and `editor.*` keys; added `viewer.enterFocus` (en, fi). Other locales fall back to English. Also adds the `ContextMenu` key and the `Shift+F` description to `docs/KEYBOARD_SHORTCUTS.md`. Salvaged from closed Jules PR #2. Verified as above. | Claude (Sonnet 5.5) / `agent/P-023-024-a11y` / `../postilaatikko-a11y` | Done (v0.3.13); commit `fe4a7bf` |
| P-017 | Medium | Prepare the Gmail API integration contract and OAuth scope plan. Plan: `docs/plans/2026-10-24-gmail-api-contract.md`. Documentation only; unblocks P-010 and P-019. | Claude (Sonnet 5.5) / `agent/P-017-gmail-api-contract` / `../postilaatikko-p017` | Done (v0.3.13); commit `b234b7c` |
| P-026 | Medium | Make keyboard shortcuts discoverable in the GUI: a shortcut registry as the single source of truth, a `?` reference overlay reachable from the UI, and shortcut hints on existing action tooltips. | Claude Opus / `agent/P-026-shortcut-discoverability` / `../postilaatikko-p026` | Done (v0.3.13); commit `dfd9417`. Verified: `npm ci`, `npm run lint`, `npm run check` (svelte-check 0 errors + registry drift test 7/7), `npm run build`. Not verified: running GUI, real key events, macOS rendering. Registry entries carry action id, label and icon so a future contextual toolbar can reuse them. |
| P-027 | Medium | Command panel (Alfred-like quick launcher with ⌘P/Ctrl+P), stabilize keyboard shortcuts dialog height to prevent focus loss on search, mark-as-read configurable delay (immediate, 1-5s, on-leave, manual), fix Portuguese i18n remnants (Caixa de entrada, selecionadas), and fix sidebar all-folders subfolder expanding and selection. | Antigravity / `agent/P-027-command-panel` / `/private/tmp/postilaatikko-p027` | Review; commits `9393dc7`, `2d608c0`. Verified: `go test -v ./internal/settings/...`, `npm run lint`, `npm run check` (svelte-check 0 errors + 7/7 registry tests), `npm run build`. |

## Queued sequential follow-up

| ID | Effort | Task | Owner / branch / worktree | Status |
| --- | --- | --- | --- | --- |
| P-006 | Medium | **High user priority:** persist include/exclude folders, read/unread, replied/unreplied, and 7-day / 30-day / all-time criteria across accounts. Apply identical criteria to the conversation list and virtual-folder search; remove hard-coded unread-only behavior, and include all statuses when no status is selected. | GPT-6.1-Sol (medium), after P-011 / `agent/p006-virtual-folder-criteria` / `/private/tmp/postilaatikko-p006` | Queued; checkpoint `65019b0` |

## Next priority after the active tasks

1. **P-017 then P-010 — Gmail API (Medium + Large):** establish the OAuth/provider contract, then implement Gmail server-side search.

## Unclaimed

| ID | Effort | Task | Dependencies / acceptance criteria |
| --- | --- | --- | --- |
| P-007 | Medium | Keep the all-folder browser directly accessible from the sidebar and let users pin folders for one-click access. | Persist pins and support folders across accounts. |
| P-008 | Medium | Let users reorder sidebar items and hide selected items. | Persist order and visibility; keep hidden items recoverable in sidebar settings. |
| P-009 | Medium | Integrate macOS Contacts as a read-only source for browsing and recipient lookup/autocomplete. | Request Contacts permission when needed. Provide an action to open the selected person in Contacts for edits; do not write contact changes from Postilaatikko. |
| P-013 | Large | Support multiple independent mail windows. | Preserve per-window navigation and selection state; detached compose already exists. |
| P-015 | Medium | Select signatures using sender, recipient, and message type. | Reuse configured sender identities and signatures; keep a manual override in the composer. |
| P-016 | Medium | Add optional per-sender and rule-based notification controls. | Keep configurable and lower priority than core mail workflows. |
| P-020 | Medium | Add full email templates to the composer. | Save and insert reusable subject/body templates; preserve recipient, sender identity, and unsaved draft fields when inserting. Keep separate from short quick replies in P-018. |
| P-021 | Medium | Add Gmail-style keyboard actions for message lists and reading. | Start with navigation, selection, archive, star, move, delete, spam, reply/forward, and undo. Support actions in the open-message view too. Disable single-key bindings in compose and other editable fields; retain standard macOS text-editing shortcuts. |
| P-022 | Medium | Recognize date/time mentions in email and offer an “Add to Calendar” action. | Depends on P-012. Parse only when confidence is sufficient, handle timezone ambiguity conservatively, and open the resulting ICS event for review/import rather than silently adding it. |
