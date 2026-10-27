# GLOBAL-002 checkpoint

Base: origin/main `3d000be`. Remote audit found GLOBAL-001 integrated and the additional checked Rollback/SMTP Abort cleanup; this branch retains both. UX-007/008/009 are already merged; their old dispatch reservation was stale and is corrected here.

Implemented:

- Gmail threads.list pages fully enumerated and deduplicated; estimates never become exact counts. Membership and flag sets are separate, preserving status matches on different messages in one conversation. Latest activity and reply predicates use bounded header workers, never message bodies. Repeated tokens, cancellation and provider errors cannot report completeness; threads deleted during metadata lookup are skipped.
- Per-window asynchronous count start/read/cancel APIs. View changes/unmount release jobs. Active virtual-folder lists show counting, completed total/unread, or incomplete/errors; these numbers do not alter loaded-result pagination.
- Non-Gmail search advances through every folder and decreasing UID page, tied to account/folder/query/UIDVALIDITY. Fresh envelopes/flags only; failed fetches preserve retryable continuation. Folder changes and mailbox resets require restart. No fixed first-page truncation.
- Virtual-folder query pagination stays on SearchMail rather than reverting to local FTS. Search and unfiltered view continuations are separate.

Verification:

- New protocol fixture with an in-memory IMAP server: all five matching messages reachable across three pages; mailbox epoch changes rejected; cancellation; no body/BINARY sections fetched.
- Gmail HTTP fixtures: multiple pages, inflated estimates, duplicate thread IDs, flags on different messages, reply/date headers, errors/cancellation, repeated page tokens and deletion during scan.
- App fixture: asynchronous scoped job completion, count values, cancellation/release and unknown view rejection.
- Affected Go race suites pass. All 162 frontend tests pass; type check has zero errors and two pre-existing titlebar accessibility warnings. Targeted ESLint and production build pass (existing chunk-size advisory).
- Real Chromium fixture: count job starts once, shows `23 conversations · 4 unread`, stops polling on completion, cancels/releases state on unmount; no browser errors. Fixture removed after verification.

Still open (GLOBAL-002 remains In progress):

- Fully enumerated non-Gmail **conversation** counts and consistent whole-thread criteria. Mixed-provider count jobs explicitly report incomplete rather than present cached/window totals as complete. IMAP pagination alone does not close this gate.
- Reuse/invalidation of completed counts across view visits; current jobs are ephemeral and a refresh enumerates again. Large-count quota/performance measurements and refresh coalescing.
- Broader two-account continuation/duplicate-copy acceptance and live Gmail/WKWebView testing in the published DMG. Search pages are not an atomic mailbox snapshot; intervening mailbox changes may require refresh.

No new DMG is claimed for this branch. The existing v0.3.18-test.4 installers were built from `8e3659a`, before this follow-up.

Provider semantics reference: [Google's search differences](https://developers.google.com/workspace/gmail/api/guides/filtering#differences_from_the_gmail_ui) explicitly distinguish Gmail API searches from UI thread-wide searches. Separate thread sets plus metadata evaluation are intentional.
