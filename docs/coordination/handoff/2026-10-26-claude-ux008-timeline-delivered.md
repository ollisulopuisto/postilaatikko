# Claude: UX-008 delivered, what UX-009 and lane B need to know

Date: 2026-10-26

- Each expanded card body now goes through `viewer/ConversationMessage.svelte`.
  It asks `GetReplyContent(accountId, messageId, '', false)`, shows new text and
  unclassified text, and folds quoted history and signatures behind buttons
  (`aria-expanded`, `data-block-kind`). A split that is uncertain, empty,
  failed, superseded, or for an encrypted or header-only message shows the
  original message, as before. The footer button "Show full message" is a
  recovery path; UX-009 should move it into the card header and add the
  persistent Conversation/Traditional toggle next to it.
- Channel: per-message requests use the empty channel (no supersession). A
  channel such as `reader` would make the messages of one thread supersede each
  other. Stale answers are dropped in the component instead.
- Remote images: the extractor blocks them (placeholder plus `data-original-src`).
  `EmailBody.svelte` now treats those as blocked images, shows its usual Load
  images banner and restores them when the reader chooses (`utils/blockedImages.ts`).
  This corrects the UX-006 handoff, which pointed at a restore path that the
  reader did not have. CSS `url()` backgrounds are removed by the extractor and
  are not restored; "Show full message" gives the original.
- Printing prints what is on screen, piece by piece, through each piece's
  `getPrintableHtml`; folded quotes are not printed.
- Opening position: `timelineScroll.openTimelineAtTarget` puts the first unread
  message (else the newest) near the top with a peek of the previous card, and
  corrects the position three times in the first second unless the reader scrolls.
- Viewer files: UX-008 is ready for review. After it merges, UX-009, UX-015,
  UX-017 and UX-018 follow one at a time (lane A). Lane B still must not edit
  `ConversationViewer.svelte` or `EmailBody.svelte`.
- Not wired: S/MIME and PGP messages keep the original renderer because
  extraction would need their decrypted text.
