# REL-001 release-baseline reconciliation / UX-008 ownership

Codex owns `codex/REL-001-preserve-release-baseline`, based on `edf7462`.
The user authorized continuing release/virtual-folder work while Claude owns UX-008.

The remote v0.3.16 tag peels to `73ca27a` and contains the P-011 candidate plus
message-focus shortcut patch `e30b37e`. Current main lacks that ancestry. REL-001
will reconcile the earlier P-011 checkpoint through `cba3457` on its review branch,
preserving current main's extraction service and other merged fixes.

Expected files: scheduled-send backend/helper, draft/compose services, the existing
composer and scheduling controls, `App.svelte` pane selection, en/fi strings and
generated Wails bindings. This restores the tagged implementation; it does not
declare delivery/security/live-device gates passed.

Bindings regeneration also requires constructing `appstate.UIState` in the
UI-state save adapter: the generated class now has `convertValues` for pinned
folders. This is a two-line adapter correction, without layout schema changes.

No edits to ConversationViewer, EmailBody, ConversationTimeline or ConversationMessage.
ConversationViewer props/events remain unchanged. UX-008 does not edit the composer
or App according to Claude's published ownership note. If this assumption changes,
coordinate before touching shared files.

`e30b37e` modifies EmailBody and is deliberately deferred while UX-008 owns it.
Its message-focus shortcut behavior must be preserved/reconciled after UX-008
before a replacement release is promoted. Do not restore it by overwriting the
new timeline/renderer; use a small announced follow-up patch.
