# GLOBAL-001 ownership

Codex owns global-search UI/backend, Gmail API transport routing, search and
OAuth scopes, All folders filtering, and automatic header refresh. Worktree
/private/tmp/postilaatikko-global001, branch codex/GLOBAL-001-api-search.
Started from origin/main plus published reader-test.3 ancestry. No main push.

User decisions: prefer Gmail API for all Gmail operations; request extra OAuth
permissions when needed; global search spans multiple accounts/all folders by
default; opening folders refreshes headers/flags without asking; refresh headers
in background; keep bodies and attachments on demand except explicit body
autodownload preferences. Also add All folders drawer search.

Sequence: inventory existing operations and token paths; shared API-first
capability/error routing; global metadata search + account scopes + pagination +
partial errors/cancellation + result opening; folder search; background header
refresh; migrate remaining operations incrementally with tests. Mutating
operations must not fall back after ambiguous acceptance; cross-account moves
confirm destination before removing source. Track remaining operation routes in
TASKS and the implementation plan; do not label the migration complete early.

## Folder search slice

Implemented account-spanning move/copy folder search, grouped destination account
headings matching the user's example, explicit source-to-destination labels for
foreign accounts, and All folders drawer search including collapsed descendants.
Search matches account name/email and folder path; keeps existing destination
folder IDs and backend copy-before-delete semantics. Results are capped at 200
rendered rows with a refinement hint. Partial account metadata failures are shown.

Verified: eight folder-search tests; frontend check (zero errors, two existing
warnings); targeted ESLint; production build; Chromium mock verifies two account
groups, cross-account indicator, keyboard destination routing, account-name search,
and Enter on Cancel does not perform a move. Native/live moves not exercised.
No Gmail transport migration or global message search is delivered by this slice.
