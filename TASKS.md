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

## Unclaimed

| ID | Effort | Task | Dependencies / acceptance criteria |
| --- | --- | --- | --- |
| P-006 | Medium | Virtual-folder include/exclude folders, read/unread and replied/unreplied status, and 7-day / 30-day / all-time windows. | Persist criteria and query them across accounts; virtual-folder search must honor the selected criteria. |
| P-007 | Medium | Keep the all-folder browser directly accessible from the sidebar and let users pin folders for one-click access. | Persist pins and support folders across accounts. |
| P-008 | Medium | Let users reorder sidebar items and hide selected items. | Persist order and visibility; keep hidden items recoverable in sidebar settings. |
| P-009 | Medium | Integrate macOS Contacts as a source for browsing and recipient lookup/autocomplete. | Requires macOS Contacts permission; write-back vs read-only scope still needs a product decision. |
| P-010 | Large | Add Gmail server-side search while retaining the IMAP search path for other providers. | Use Gmail's `messages.list` query support; handle OAuth scope and result paging. The `q` parameter is not available with `gmail.metadata`. |
| P-011 | Large | Implement reliable scheduled send with the always-on Mac Studio helper. | Durable queue, due-time delivery, retry/recovery, cancellation, and delivery status. |
| P-012 | Medium | Recognize dates and times in email and link them to installed calendar apps. | Build on the Calendar extension; agree on event/open behavior before UI work. |
| P-013 | Large | Support multiple independent mail windows. | Preserve per-window navigation and selection state; detached compose already exists. |
| P-015 | Medium | Add contextual signatures, quick replies, and full email templates. | Reuse existing sender identities and per-identity signatures. |
| P-016 | Medium | Add optional per-sender and rule-based notification controls. | Keep configurable and lower priority than core mail workflows. |
