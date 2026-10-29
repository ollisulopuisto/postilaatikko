# GLOBAL-002 recovery checkpoint

Base: fetched `origin/main` at `1817a78` (2026-10-29). New isolated branch `codex/GLOBAL-002-online-counts-resume`, worktree `/private/tmp/postilaatikko-global002-resume`. Existing `codex/GLOBAL-002-online-counts` at `e9f9be5` is preserved. The old worktree is missing: its uncommitted non-Gmail thread-count/header changes were not recovered.

Reapplied committed count jobs/UI and IMAP decreasing-UID continuation from `e9f9be5`, preserving current main's Gmail shared quota budget, thread-list implementation, saved-search virtual-folder query, cached first paint and cache refresh timestamp. Only one `Threads` implementation remains.

The requested durable WIP checkpoint was pushed as `fa03f94`; the verified reconciliation is `581c6f8`. Follow-up reconciliation fixes saved-query count membership, duplicate scan progress, redundant flag enumeration when header metadata is already required, and false completeness after cancellation at the last enumeration page. Each of those cases failed before its fix and passed afterward.

Verification for the recovered slice:

- Full `go test ./...` passes after building the required frontend embed assets. An initial full-suite attempt before the frontend build failed only on the absent `frontend/dist` embed; it was rerun successfully.
- Affected Gmail/sync/app race tests pass; `go vet ./app ./internal/gmail ./internal/sync` and whitespace checks pass.
- Protocol fixture verifies 12 independent results across two non-Gmail accounts/two folders, account-qualified identities, unchanged retry continuation after a server SEARCH failure, and no body/BINARY content fetches. Existing protocol tests cover UIDVALIDITY changes, cancellation and decreasing UID pages.
- Job fixture verifies saved-query scope, successful counts, cancellation/release and unknown-view rejection. Mixed providers preserve successful Gmail totals while reporting incomplete; disabled accounts are excluded.
- All 178 frontend tests pass; Svelte check reports zero errors and the two existing titlebar ARIA warnings. Changed-component ESLint and production build pass (existing chunk-size and annotation advisories).
- No native app, live provider or current-branch browser smoke is claimed.

Remaining whole-task gates: exact non-Gmail conversation counts/whole-thread criteria (IMAP pages still return message-level matches), completed-count reuse/invalidation and coalescing (view reloads currently restart count enumeration), large-mailbox quota measurements, criteria-edit cursor invalidation, and native/live Gmail/IMAP testing. Provider pages are not an atomic mailbox snapshot; arrivals/deletions can require refresh. Unsupported count providers report incomplete. No main merge, release or DMG is claimed.
