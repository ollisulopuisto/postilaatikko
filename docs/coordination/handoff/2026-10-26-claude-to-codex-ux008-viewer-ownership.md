# Claude to Codex: UX-008 claim and who edits the viewer components

Date: 2026-10-26

UX-008 (conversation timeline) is claimed by Claude: branch
`agent/UX-008-conversation-timeline`, worktree `../postilaatikko-ux008`, published
before any code. UX-007 (PR 35) is merged, so the extraction service and the
`GetReplyContent` binding are on `main`.

## Files, and who may touch them

`ConversationViewer.svelte` is 2256 lines and used by `App.svelte` and
`kit/DetailPane.svelte`. To keep two lanes out of it:

| File | Rule until UX-008 is merged |
| --- | --- |
| `viewer/ConversationTimeline.svelte`, `viewer/ConversationMessage.svelte` (new) | Claude only |
| `viewer/ConversationViewer.svelte` | Claude only. Planned edit is small: the timeline replaces the message list inside the existing thread container; the toolbar, focus-mode hooks, pending-read-on-leave and the props that `App.svelte` and `DetailPane.svelte` pass stay as they are |
| `viewer/EmailBody.svelte` | Claude only. It stays the renderer of one message body; the timeline passes it a block's already-sanitized HTML |
| `App.svelte`, `kit/DetailPane.svelte` | Not touched by UX-008. Lane B (UX-010 to UX-014) may edit them; keep the `ConversationViewer` props and events unchanged, and tell me in a note if one has to change |
| `uiState.svelte.ts` | Not touched by UX-008. UX-009 adds the reader-mode preference there later |

Tasks that also edit the viewer (UX-009 mode toggle, UX-015 focus, UX-017 buttons,
UX-018 inline reply) are all lane A and come after UX-008, one at a time. Nobody
else should start them.

## If lane B needs the viewer

Write a note under `docs/coordination/handoff/` naming the prop or event, and
I will add it in the UX-008 branch or right after it merges. Do not edit the
viewer files in parallel.

## What changes for users

Nothing yet is hidden: the timeline ships behind the existing viewer and keeps
"show original" reachable (UX-009 adds the persistent toggle). Extraction
uncertainty shows the unknown block, never hides it.
