# Dispatch board

Who works on which part of the D-002 plan
(`docs/plans/2026-10-26-conversation-navigation-agent-api.md`, task rows in
`TASKS.md`), and in what order. Protocol: [README.md](README.md).

Last updated by the coordinator: 2026-10-26. The assignments below are a
proposal until the user confirms them.

## Lanes

Lanes group tasks that edit the same files, so two agents never work in the same
files at once.

| Lane | Tasks (in dependency order) | Main files | Proposed owner |
| --- | --- | --- | --- |
| A. Reply extraction and reader | UX-004, UX-005, UX-006, UX-007, UX-008, UX-009, UX-015, UX-018 | `internal/email/replycontent/`, `internal/email/`, `frontend/src/lib/components/` (reader, thread view), composer drafts | Claude |
| B. Navigation and layout | UX-010, UX-011, UX-012, UX-013, UX-014, UX-033, UX-029 | `frontend/src/lib/components/` (sidebar, panes), `frontend/src/lib/layout/`, `uiState.svelte.ts` | Codex / ChatGPT |
| C. Actions and agent API | UX-016, UX-019, UX-020, UX-021, UX-022, UX-023, UX-024, UX-037, UX-038 | `internal/` (new API packages), action layer, docs under `docs/api/` | Codex / ChatGPT, starting with the design task UX-019 |
| D. Shared query path | UX-001, UX-003 | `internal/` store and virtual-folder code, `internal/views/` | Coordinator decides; UX-001 is an integration checkpoint |
| Later | UX-017, UX-025 to UX-028, UX-030 to UX-032, UX-034 to UX-036, UX-039 to UX-041 | | Assigned when their dependencies are merged |

Lane D is a shared critical path: UX-003 blocks UX-013, UX-021 and UX-028.
UX-007 depends on UX-002, UX-005 and UX-006: extraction/cache uses existing body
fetching and does not require folder-refresh UI or the live mailbox checkpoint.
Keep the remaining UX-001 refresh/live-device gate explicit for its own dependants.

## Status

Updated when a pull request merges.

| Task | State |
| --- | --- |
| UX-002 view contract | Merged (PR 29) |
| UX-004 extraction schema and fixtures | Merged (PR #30, `664f630`); subsequent additive parser warnings/fixtures merged in PRs #33/#34 |
| UX-005 plain-text extraction | Merged (PR #33, `74ed7ff`) |
| UX-006 HTML extraction | Merged after UX-005 (PR #34, `7436026`) |
| UX-007 extraction service/cache | Reserved for Claude per user handoff; owner publishes branch/worktree before implementing |
| UX-001 integration checkpoint | PR #32 merged (`2191cb8`): date/NULL-subject query repair and inventory. Review gate remains for live-device verification and remaining header-refresh/exclude-mode integration |

## Next up

Take the first task in your lane whose dependencies are merged and which has no
branch on `origin`.

- **Lane A (Claude):** UX-007 service/cache is next and reserved. Fetch current
  main with both PRs #33/#34 before starting; use the accepted extraction APIs
  in the handoff notes. UX-006 now depends on UX-005 shared helpers; the old
  parallel-parser assumption is superseded. UX-008 reader waits for UX-007.
- **Lane B (Codex):** UX-010 typed navigation entries, after UX-001.
- **Lane C (Codex):** UX-019 versioned agent API design, which needs only UX-002.
  UX-016 shared action targets, also UX-002 only, goes to whichever of lane C or
  lane A has capacity; ask before taking it.
- **Lane D:** coordinate UX-001's remaining header-refresh/exclude-mode work with
  the navigation owner and retain live-device verification. UX-003/UX-010 must
  not treat the full UX-001 gate as passed merely because PR #32 merged.

## File ownership while tasks are active

A file named here has one active owner. To change it, write a handoff note first.

| Files | Owner |
| --- | --- |
| `internal/views/`, `frontend/src/lib/views/` | Merged (UX-002); changes through a handoff note |
| `internal/email/replycontent/` | UX-005/006 merged; Claude owns UX-007 service/cache. Changes to shared model/parser behavior need a handoff and regression fixtures |
| `frontend/src/lib/layout/`, sidebar components | Lane B owner |
