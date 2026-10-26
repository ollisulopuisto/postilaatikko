# Claude: UX-009 reader mode, notes for the next viewer tasks

Date: 2026-10-26

- Reader mode (`conversation` or `traditional`) lives in the UI state
  (`uiState.svelte.ts` `getReaderMode`/`setReaderMode`, `appstate.UIState.ReaderMode`,
  omitted when unset). Unknown stored values mean conversation.
- `ReaderModeToggle.svelte` sits at the start of the viewer's right toolbar group.
  `ConversationMessage` takes `mode`; in traditional it makes no extraction
  request and draws the original body.
- Switching keeps the place: `captureScrollAnchor` / `holdScrollAnchor` in
  `timelineScroll.ts` keep the card nearest the top where it was while bodies
  change height, and release as soon as the reader scrolls. UX-015 and UX-018
  can reuse them for focus restore and for inserting the inline reply box.
- "Show original" / "Show new text only" sits at the top of each split card.
- This PR is based on the UX-008 branch (PR 36); merge 36 first. After both,
  UX-015, UX-017 and UX-018 are the next viewer tasks (lane A, one at a time).
