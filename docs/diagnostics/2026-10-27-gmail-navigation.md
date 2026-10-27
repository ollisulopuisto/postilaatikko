# Gmail/navigation delivery candidate

Implementation: dc08165. Branch: codex/GLOBAL-001-api-search. Worktree: /private/tmp/postilaatikko-global001.
Based on origin/main plus the published reader-test.3 integration ancestry.
No main push or merge.

## Implemented

- Versioned typed card preference persisted in UIState; custom groups, drag and keyboard entry/group movement, collapse, hide/restore and quick favorites (Cmd/Ctrl+1…0). All Inboxes is a Virtual folders entry; Contacts/Calendar rail entries default off.
- Folder search across account names/emails/paths, including collapsed branches; move/copy account groups and explicit foreign-account labels (2d86d01).
- Gmail REST metadata/raw/full body/attachment/thread/history/label/draft/send/insert/delete operations. Non-idempotent send, draft create and import do not automatically retry transport failures or fall back to another sending protocol. MIME API upload preserves SMTP-envelope-only Bcc recipients.
- Migration 54 retains explicit provider references. The legacy schema's reserved numeric cache key is never used as an IMAP UID. Native message identities are account-qualified and stable across metadata updates; bodies are preserved.
- Gmail metadata/history sync projects label membership into the cache, advances bounded backfill and updates server label counts. Normal body reads leave file attachments remote; crypto MIME retains raw parsing. New local cache keys cannot fall through to IMAP raw/body/flag/copy operations.
- Gmail flags, same-account label moves/copy/archive/spam/trash/delete, send/read receipts, draft sync and helper sending use REST first. API-confirmed cross-account imports precede source trash; ambiguous or unconfirmed copies preserve the source. Move/undo works without IMAP.
- SearchMail provides account-qualified global/virtual pages, server query scope, bounded concurrency/cursors, cancellation, partial account errors and reauthorization links. Virtual statuses use thread metadata; dates use latest activity and reply detection uses sent In-Reply-To metadata. Gmail result opening hydrates the metadata thread and uses existing explicit content rendering.
- Folder opening starts a coalesced metadata refresh. It does not restart/cancel manual work. Background non-Gmail sync visits all folders by default; Gmail uses one account-wide metadata/history pass instead of downloading identical headers separately for each label.

## Verification

- All Go packages pass; affected race suites pass. Native empty-cache/no-IMAP HTTP fixture proves header-only sync, body-on-open and separate file download.
- Native history fixture also verifies removed labels disappear without body/file reads. Two-account HTTP fixture covers server-only virtual/global results, account ID isolation, cursor scope/paging, partial authorization failure, native move/undo and source preservation after unconfirmed destination import.
- REST tests cover metadata/labels/raw, Bcc preservation and one request only after ambiguous send/import failures. Store/migration tests preserve body content and reject cross-account provider lookups.
- Frontend type check: zero errors, two pre-existing titlebar warnings. Targeted lint passes. All 162 frontend Node tests pass. Browser smoke covers real card/store components, drag/drop, custom groups, saved collapse/hide state, Cmd+1, global scope choice and account-qualified result opening. Production compilation passes.
- Actual installed-profile data was not available in this environment (the default Aerion DB had no tables). Live Google, real provider MIME variants, native window behavior and reconnect/authorization must be tested on the mail device.

## Explicit limits

- Not merged to main. A successful installer is a test candidate, not proof of live mailbox fidelity.
- IMAP IDLE remains a notification hint; subsequent Gmail reads/mutations prefer REST. Password-only Gmail accounts retain IMAP until configured with Google OAuth. Explicit IMAP/SMTP configuration tests remain protocol tests.
- Non-Gmail cross-folder search uses bounded IMAP result windows and reports that limit. Native Gmail search pages remain server-backed. Gmail native query semantics apply (no promise of Gmail web UI alias expansion).
- The old cached count/unread-badge endpoints do not represent complete online totals. Active virtual views use cursor continuation rather than claiming a complete count. Very large search sessions bound the cursor dedup set; the UI deduplicates loaded account/thread IDs.
- Legacy GetVirtualFolderConversations is adapted to the server service; full cached-count/shared-query unification and the separate UX drawer Keep-open/API/MCP tasks remain follow-ups.
- P-011 live/security/recovery gates remain. Changed remote scheduled drafts are retained if exact source fingerprint/body validation cannot be proven.
