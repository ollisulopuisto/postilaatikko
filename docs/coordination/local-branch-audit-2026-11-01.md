# Local branch audit — 2026-11-01

This is the disposition of every local branch after the coordinator cleanup.
Local `main` includes the current `origin/main` plus reviewed release candidates
and coordinator commits; it has not been pushed. Local-only changes to this audit,
`TASKS.md`, and `ROADMAP.md` are being committed separately. The untracked
`tags.png` is user data and was left untouched.

## Kept: active or reviewable work

| Branch | Disposition |
| --- | --- |
| `agent/P-021-viewer-shortcuts` | PR #86 is merged; local branch deleted. Its clean worktree is detached at main to preserve untracked `frontend/node_modules`. |
| `codex/P043-close-out` | Keep until the documentation close-out commit `666e9f7` is integrated into local main; P-043 behavior is already present in main. |
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
- `agent/P-043-tabbed-mail-workspaces` — its older implementation was fully superseded by current main: tabbed workspaces (`76c1cd0`), detached search/thread state (`af35980`), active-tab navigation and folder browser (`011a3f6`), and instant reader-tab/macOS detach behavior (`9c3dffc`, `3b917fb`, `a41f682`). The branch had five unique commit objects, but no remaining user-visible behavior absent from main; no code was cherry-picked to avoid regressing later UI changes. Local branch retired after this comparison; remote branch left untouched.
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

P-043 close-out: current main initializes detached mail workspaces from the selected thread and restores the search query (`app/detached_reader.go`, `app/mail_window.go`, `main.go`, `frontend/src/MailWindowApp.svelte`, `frontend/src/lib/windows/mailWindow.ts`, and `MessageList.svelte`). The task's requested behavior is complete; the retired `agent/P-043-tabbed-mail-workspaces` branch was redundant history, not missing implementation. The documentation-only close-out commit was integrated into local main on 2026-11-02; its local branch and worktree were removed.

The local `main` branch is intentionally ahead of `origin/main`; this cleanup
does not publish or release those commits. No remaining local branch is
unclassified: all kept branches appear above, and the listed retired branches
were removed.


## Follow-up — 2026-11-02

Local `main` was fast-forwarded from its prior checkpoint to `origin/main`, then advanced through isolated integration branch `codex/merge-worktrees-2026-11-02` (now deleted). No push or release was made.

Integrated in the follow-up:

- P-055 wide-email fitting and P-059 duplicate-path removal in the move picker. Source-branch frontend build/diff checks passed for P-055; the live WKWebView/real-message check remains. P-059 had a clean diff check; tests were not run.
- NAP-002 Undo Send cancellation cleanup and NAP-003 unasserted IMAP IDLE. Their source branches record passing Go regression suites; native Activity Monitor verification remains.
- P-057’s agreed scope and P-043’s documentation close-out.
- P-021 and KBD-001 behavior were already present on origin/main (PR #86 and `ec8e3ac` respectively); task rows now record the integration state.

Retired local branches after checking integration or supersession: `codex/CMD-001-command-catalog`, `codex/KBD-002-forward-delete`, `agent/P-021-viewer-shortcuts`, `codex/KEYBOARD-COPY-001`, `codex/NAP-002-app-nap-tests`, `codex/NAP-003-unasserted-imap-idle`, `codex/P-055-responsive-email`, `codex/P-057-smart-move-rules`, `codex/P-058-inbox-body-prefetch`, `codex/P-059-folder-picker-path-label`, `codex/P043-close-out`, `codex/RELEASE-2026.11.01`, `codex/RELEASE-2026.11.01.1`, `codex/RELEASE-2026.11.01.2`, and the integration branch. Release tags are retained; remote branches were not deleted.

Retained review/active work: `codex/CI-003-on-demand-macos-test-build`, `codex/FACET-001-visible-search-facets`, `codex/P-016-smart-notifications`, `codex/P-020-email-templates`, `codex/P-054-sidebar-body-prefetch`, `codex/UX-020-local-api-listener`, and `codex/UX-042-reply-all-safety`. Also retained are the GLOBAL-002 recovery branch, archived P-037/P-038/P-039 branches, and the 2026.10.31 release-evidence branches. These were left intact because they are incomplete, under review, or preserve release/recovery evidence.

The detached P-021 worktree remains only to preserve its untracked `frontend/node_modules`; the primary checkout’s untracked `tags.png` also remains untouched. Local `main` is ahead of `origin/main` and remains unpublished.
