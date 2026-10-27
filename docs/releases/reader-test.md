Postilaatikko 0.3.18 Gmail body/quota repair test build, built on GitHub.
Choose arm64 for Apple Silicon (including Mac Studio), amd64 for Intel.
Open the DMG and drag Postilaatikko onto Applications.

- Repairs Gmail messages whose headers were incorrectly marked as already downloaded. Existing affected empty native cache rows are repaired automatically so opening the message requests its body. Downloaded content is preserved.
- Gmail search lists threads directly instead of fetching the same message metadata twice. Background header pages are smaller; a shared account read budget paces current quota-unit costs, and a server quota response pauses other reads rather than multiplying immediate retries.
- Body download failures keep the message visible with Retry. A virtual-folder provider error is shown as an error, with existing results retained for that view, rather than reported as an empty mailbox.
- Organize sidebar cards: reorder, collapse, hide/restore, custom groups and folder moves. Star up to ten entries for the quick rail and Cmd/Ctrl+1…0. Contacts/Calendar shortcuts default off.
- Search all mail across accounts from the sidebar or `/`. Gmail uses native server queries; choose an account or search all enabled accounts. Account errors remain visible alongside available results.
- Virtual folders query Gmail server metadata directly, including folder unions/exclusions, read/star/reply status and latest-activity windows. They no longer depend on an already-populated IMAP cache. The selected row stays put after its own read transition.
- Gmail OAuth reads and mail operations prefer Gmail API. Includes drafts, sending, native label actions, body/source/file reads and scheduled-helper delivery; preserves Bcc and copy-before-source-removal safeguards.
- Automatic header refresh on folder open and bounded account-wide Gmail history/backfill in the background. Normal message bodies and file attachments are downloaded separately, on demand.
- All folders and move/copy searches span accounts; move destinations are grouped by account and crossing the account boundary is explicit.
- Includes the previous compact reader, contrast/signature/reaction fixes and detached reader windows.

Verified with full Go tests, affected race tests, frontend checks and real Chromium fixtures. Live Google authorization/provider behavior and native WKWebView remain device checks. Gmail API must be enabled for the OAuth project; reconnect when the app requests additional permissions. Non-Gmail providers retain IMAP; their cross-folder search currently returns bounded windows with an explicit limitation notice. Legacy cached count endpoints are not server-complete totals; active virtual views use cursor pagination.

Cache schema advances to 55; older binaries reject newer caches. Send Later's existing live delivery/pairing/recovery acceptance gates remain open. This is a prerelease from the isolated integration branch, not a main/stable promotion.
