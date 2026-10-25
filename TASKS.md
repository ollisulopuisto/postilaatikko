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
| P-001 | Easy | Add Finnish core and extension translations, locale registration, and key/placeholder parity. | Coordinator / `main` primary checkout | In progress via AGY CLI |
| P-002 | Easy | Add macOS Preferences shortcut and title-bar double-click maximize; show Postilaatikko in visible window chrome. | Coordinator / `main` primary checkout | In progress |
| P-003 | Easy | Prefer account display names under Inbox and in account rows, falling back to email. | Coordinator / `main` primary checkout | In progress |
| P-004 | Easy | Keep Edit Account dialog size stable across tabs and anchor the footer. | Coordinator / `main` primary checkout | In progress |
| P-005 | Medium | Make routine sync header-only; fetch message bodies and attachment metadata when a message is opened. | Coordinator / `main` primary checkout | In progress |

## In progress in separate worktrees

| ID | Effort | Task | Owner / branch / worktree | Status |
| --- | --- | --- | --- | --- |
| P-014 | Medium | Add local cross-account filters for routing incoming messages. | opencode (Qwen3.8-27B) / `feat/p014-cross-account-filters` / `../postilaatikko-p014` | Review |
| P-011 | Large | Critical E2EE Send Later: durable queue, pair clients with the Mac Studio helper, encrypt queued messages to the helper public key, and keep the private key and sending credentials on the helper. Relay stores ciphertext only; helper decrypts at delivery time. Include retry/recovery, cancellation, status, and missed due-time handling. Hosted decryption/sending is out of scope. | GPT-6.1-Sol (medium) / `agent/p011-e2ee-scheduled-send` / `/private/tmp/postilaatikko-p011` | In progress; checkpoint `7be5fe3` |
| P-012 | Easy | ICS event handoff (Easy): create an ICS event from structured date/time details and open it for review/import. | OpenCode / `agent/p012-ics-event-handoff` / worktree `../postilaatikko-p012` | Done |
| P-018 | Easy | Reusable quick-reply snippets in the composer: `QuickReplies.svelte` (footer dropdown + manage dialog), local storage in the settings table, Go store tests. Verified: `go test ./internal/settings`, `svelte-check` (no new errors), `vite build`. Not verified: `app` package compile (GTK headers unavailable in this environment), Wails bindings were hand-edited to match generator output, behavior in the running app. | Claude (Sonnet 5.5) / `agent/P-018-quick-replies` / `../postilaatikko-p018` | Review; commit `7e0f935` |
| P-017 | Medium | Prepare the Gmail API integration contract and OAuth scope plan. Plan: `docs/plans/2026-10-24-gmail-api-contract.md`. Documentation only; unblocks P-010 and P-019. | Claude (Sonnet 5.5) / `agent/P-017-gmail-api-contract` / `../postilaatikko-p017` | Review; commit `b234b7c` |

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
| P-010 | Large | Add Gmail server-side search while retaining the IMAP search path for other providers. | Depends on P-017. Use Gmail's `messages.list` query support; handle OAuth scope and result paging. The `q` parameter is not available with `gmail.metadata`. |
| P-013 | Large | Support multiple independent mail windows. | Preserve per-window navigation and selection state; detached compose already exists. |
| P-015 | Medium | Select signatures using sender, recipient, and message type. | Reuse configured sender identities and signatures; keep a manual override in the composer. |
| P-016 | Medium | Add optional per-sender and rule-based notification controls. | Keep configurable and lower priority than core mail workflows. |
| P-017 | Medium | Prepare the Gmail API integration contract and OAuth scope plan. | Inspect current account/token storage and provider interfaces; document the smallest scopes needed for search and Gmail-specific actions, token refresh/error handling, and an IMAP fallback. No credential migration or broad auth rewrite in this task. Unblocks P-010 and P-019. |
| P-018 | Easy | Add reusable quick-reply snippets to the composer. | Create, edit, delete, and insert plain-text replies without losing the draft or current sender identity. Keep the first version local to this installation. |
| P-019 | Medium | Add Gmail-native mark-as-spam and undo actions. | Depends on P-017. Use Gmail's native spam label/action for Gmail accounts, preserve the existing provider-neutral path, and expose undo with clear success/failure feedback. |
| P-020 | Medium | Add full email templates to the composer. | Save and insert reusable subject/body templates; preserve recipient, sender identity, and unsaved draft fields when inserting. Keep separate from short quick replies in P-018. |
| P-021 | Medium | Add Gmail-style keyboard actions for message lists and reading. | Start with navigation, selection, archive, star, move, delete, spam, reply/forward, and undo. Support actions in the open-message view too. Disable single-key bindings in compose and other editable fields; retain standard macOS text-editing shortcuts. |
| P-022 | Medium | Recognize date/time mentions in email and offer an “Add to Calendar” action. | Depends on P-012. Parse only when confidence is sufficient, handle timezone ambiguity conservatively, and open the resulting ICS event for review/import rather than silently adding it. |
| P-023 | Easy | Add accessible names to icon-only controls in the Contacts list. | In `extensions/contacts/frontend/components/ContactList.svelte`, add `aria-label` values to search-clear, search-toggle, sort, and add-contact buttons, reusing their existing localized title strings. No behavior changes. |
| P-024 | Easy | Localize hard-coded accessibility labels in the shared UI kit. | Replace English-only labels in `kit/DetailOverlay.svelte` and `kit/RichTextEditor.svelte` with i18n keys, adding translations while preserving locale key parity. Keep the change scoped to labels and help text. |
| P-025 | Easy | Audit the keyboard shortcut reference against current behavior. | Compare `docs/KEYBOARD_SHORTCUTS.md` with the existing shortcut definitions and correct stale or platform-inaccurate descriptions. Documentation only; do not add or change key bindings. |
