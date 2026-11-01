# Local branch audit — 2026-11-01

This is the disposition of every local branch after the coordinator cleanup.
Local `main` includes the current `origin/main` plus five reviewed release
candidate commits; it has not been pushed. Local-only changes to this audit,
`TASKS.md`, and `ROADMAP.md` are being committed separately. The untracked
`tags.png` is user data and was left untouched.

## Kept: active or reviewable work

| Branch | Disposition |
| --- | --- |
| `agent/P-021-viewer-shortcuts` | Keep with its worktree. Open PR #86; review/fix the current lint failure and retain the live macOS verification gate. |
| `agent/P-043-tabbed-mail-workspaces` | Keep branch; worktree was clean and removed. Five unique commits remain outside main. Automatic review blocked deleting it because doing so risked losing unmerged implementation. Reassess its unique patches before any retirement. |
| `codex/CI-003-on-demand-macos-test-build` | Keep with active worktree; CI optimization task. |
| `codex/FACET-001-visible-search-facets` | Keep with active worktree; search UI task. |
| `codex/GLOBAL-002-online-counts` | Keep as an older recovery checkpoint; incomplete follow-up exists remotely. |
| `codex/NAP-002-app-nap-tests` | Keep with active worktree. |
| `codex/NAP-003-unasserted-imap-idle` | Keep with active worktree. |
| `codex/P-016-smart-notifications` | Keep with active worktree. |
| `codex/P-020-email-templates` | Keep with active worktree. |
| `codex/P-054-sidebar-body-prefetch` | Keep with active worktree. |
| `codex/P-055-responsive-email` | Keep with active worktree. |
| `codex/P-057-smart-move-rules` | Keep with active worktree. |
| `codex/UX-020-local-api-listener` | Keep with active worktree. |
| `codex/UX-042-reply-all-safety` | Keep with active worktree. |

## Kept: release evidence and recovery history

| Branch | Disposition |
| --- | --- |
| `codex/RELEASE-2026.10.31` | Keep as a release/version and local-build-script checkpoint. |
| `codex/RELEASE-2026.10.31-latest` | Keep with its worktree and arm64 DMG artifact for audit/retrieval. The DMG predates the current main and is not represented as a current build. |
| `archive/2026-11/P-037-virtual-folder-empty` | Keep archived diagnostic evidence. |
| `archive/2026-11/P-038-sidebar-groups-rail` | Keep archived diagnostic evidence. |
| `archive/2026-11/P-039-virtual-folder-followup` | Keep archived diagnostic evidence. |
| `archive/2026-11/integration-p037-p038` | Keep archived integration history. |
| `archive/2026-11/integration-p037-review` | Keep archived integration review history. |

## Retired local branches

These branches were deleted only after checking their commits against main or
their superseding PR/state:

- `agent/P-015-contextual-signatures` — integrated through PR #72.
- `agent/P-030-bulk-message-actions` — superseded by the integrated main state.
- `codex/P-045-virtual-folder-picker`, `codex/P-046-folder-picker-tree-hotkeys` — integrated through PR #72.
- `agent/UX-040-folder-content-policy`, `agent/UX-041-folder-content-controls`, `agent/UX-043-folder-quick-filters` — superseded/integrated through PR #72.
- `codex/UX-001-finish-virtual-folder`, `codex/UX-019-agent-api-design` — integrated or superseded by later API work on main.
- `codex/UX-026-queue-api-recovery` — recovery branch superseded by integrated Reply Later API work.
- `codex/P-051-message-list-folder-tags`, `codex/P-052-instant-reader-tabs`, `codex/P-053-favorite-order` — their reviewed release-candidate commits are integrated into local main (`bf8bc77`, `9c3dffc`, `3b917fb`, `5d51dbc`, `a41f682`).
- `codex/integrate-latest-ui` — integration branch fast-forwarded into local main.
- `agent/P-026-llm-interface` — its durable privacy/approval principle is recorded in `ROADMAP.md`; the remaining branch changes were stale task-board edits.
- `agent/p007-sidebar-favorites`, `agent/p008-sidebar-groups`, `agent/p029-sidebar-app-visibility` — sidebar cards, quick access, groups, and settings are already present in main.
- `agent/p023-contacts-aria-labels` — the same accessible labels are already present in the Contacts component on main.
- `codex/PR-077-repair` — documentation-only reconciliation for closed/superseded PR #77.
- `codex/INT-DISPATCH-UX008-update` — stale coordination snapshot; its TASKS rewrite would discard unrelated task history, so it was not merged.

The local `main` branch is intentionally ahead of `origin/main`; this cleanup
does not publish or release those commits. No remaining local branch is
unclassified: all kept branches appear above, and the listed retired branches
were removed.
