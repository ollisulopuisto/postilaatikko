# Complete online counts and search pagination

**Goal:** Report fully enumerated online virtual-folder conversation/unread counts and allow non-Gmail cross-account search to continue beyond its first folder window, without downloading bodies or pretending provider estimates are exact.

**Base:** main 3d000be includes GLOBAL-001 plus checked cleanup return values. No open PRs or published competing GLOBAL-002 branch existed at claim time. Retain those fixes.

**Architecture:** Gmail threads.list enumerates unique server thread IDs. Membership, unread, starred and recent-activity queries are evaluated as separate thread sets, then intersected/subtracted so flags or recent replies on different messages in a thread still match. Reply-status criteria use bounded thread-header reads. Counts run asynchronously with progress, explicit completeness, cancellation, cached completed results and invalidation. Never label Gmail resultSizeEstimate as an exact count. IMAP pages use server UID continuations bound to account/folder/UIDVALIDITY; headers/envelopes only. One shared cursor carries per-provider continuation state; errors remain per-account.

## Steps

1. Gmail REST thread pages (`internal/gmail/native.go`, HTTP tests): add threads.list with native query, page token and includeSpamTrash. Test multi-page ID enumeration and estimates not treated as completion.
2. Count set semantics (`internal/sync` or a small new query package, table tests): membership union/exclude, all/read/unread, starred/unstarred, latest-activity intersections and replied/unreplied metadata. Freeze the date cutoff for each count job. Deduplicate account-qualified thread IDs. Cancellation and bounded concurrency are required.
3. App count job (`app/virtual_folder.go` plus new count service): start/read/cancel, report running/complete/failed and scanned progress, return exact totals only after enumeration. Preserve previous completed counts while refreshing; scope changes invalidate them. Keep legacy cached totals explicitly separate.
4. IMAP page transport (`internal/sync/search.go`): SEARCH criteria + UID high-watermark, UIDVALIDITY validation, envelope/header page fetch, next cursor; avoid bodies and full repeated metadata fetch. Test mailbox reset, skipped/deleted UIDs, last page and cancellation.
5. Cross-account cursor (`app/mail_search.go`): thread the IMAP folder/page state through SearchMail; stop marking an account complete merely because its first bounded folder result window ended. Test two accounts, several folders, duplicate copies, partial failure and scope changes.
6. UI (`MessageList`, GlobalSearch, virtual-folder store): show progress/incomplete state and completed online counts. Load more follows provider continuation; no hidden truncation or arbitrary extra body fetch. Preserve selected-row stability and keyboard behavior.
7. Verify Go HTTP/store/race suites, full frontend tests/type/lint/build and browser interactions; commit task updates and push review branch. Do not merge main. Build a follow-up DMG only after a coherent reviewable slice is verified.

Live Google/macOS gate is separate: user must run the published v0.3.18-test.4 DMG with the configured mail accounts. No populated local mail profile was available to this agent; do not claim fixture tests close that gate.

Remaining agreed order after GLOBAL-002: Contacts/date recognition/independent mail-list windows; Send Later acceptance; drawer/Focus/inline replies; API/MCP/signatures/templates.
