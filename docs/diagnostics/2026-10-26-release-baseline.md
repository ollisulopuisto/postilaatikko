# Release baseline reconciliation (REL-001)

The remote `v0.3.16` tag peels to `73ca27a`; it is not an ancestor of
`origin/main` at `edf7462`. It contains the P-011 Send Later checkpoint through
`cba3457` and the message-focused shortcut fix `e30b37e`. Cutting a replacement
release from unchanged main would omit both behaviors present in that tag.

This branch merges the P-011 checkpoint, resolves documentation conflicts in
favor of current product/task decisions, and regenerates Wails bindings with
both scheduling and UX-007's `GetReplyContent`. Generated UIState now requires
its constructor in the save adapter; persisted fields are unchanged.

## Verification

- Race-enabled tests for scheduledsend, email/replycontent and draft pass.
- Queue regression tests cover encrypted round trips, idempotent submission,
  cancellation tombstones, stale revisions, revoked clients, unauthorized
  accounts, ciphertext tampering, and suppression of automatic resend after
  ambiguous submission or accepted delivery with a failed Sent copy.
- Go vet and Go build pass for app, helper and scheduling package.
- Frontend production build passes. Type checks pass with zero errors and two existing warnings; all seven shortcut registry tests pass. Generated bindings retain the generator’s blank-line whitespace.

Tests use temporary private queues and fake transport. They do not send mail,
read the user's accounts, or exercise macOS Keychain.

## Gates before a replacement release

- Integrate UX-008 and the visible Conversation/Traditional toggle, then perform
  a real Mac smoke test. No protected viewer files are changed here.
- Reconcile `e30b37e` after Claude releases viewer ownership. That change is
  intentionally excluded from this merge; check shortcuts with message focus.
- Keep P-011's real delivery, pairing/key management, security and recovery
  acceptance gates open. This is ancestry reconciliation, not their completion.
- Scheduled-pane navigation remains pending as documented by P-011. A successful
  build does not establish end-to-end access to every scheduling control.
- Produce the eventual DMG through GitHub's release workflow after integration;
  this branch does not publish a release or change main.
