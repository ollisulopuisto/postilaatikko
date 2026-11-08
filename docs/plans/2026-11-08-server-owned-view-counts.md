# Server-owned virtual-folder unread counts

**Goal:** Stop the connected desktop from calculating saved-view unread counts against the shared mailbox database. The helper remains authoritative for message rows and saved-view criteria.

## Tasks

1. Add an authenticated `mail.read` API route that returns the helper's existing `virtualFolderStore.UnreadCounts()` map without changing SQL/count semantics.
2. Add a typed API-client and desktop connector method.
3. Route `GetVirtualFolderUnreadCounts` through the helper when connected. Use the local store only when the configured local helper is unavailable; propagate remote/auth/configuration errors.
4. Add focused route and desktop-routing coverage, document the ownership change, update the changelog and task ledger.
5. Run focused Go tests, affected package builds, and `git diff --check`; commit implementation and ledger, fast-forward main, and push.

## Status

Implementation and focused verification are complete. Connected desktop reads use the authenticated helper endpoint; local fallback is limited to local-helper unavailability, and existing SQL/count semantics are unchanged. See HOSTED-SERVER-048 in `TASKS.md` for the commit and verification record.
