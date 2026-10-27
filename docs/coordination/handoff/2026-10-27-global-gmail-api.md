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
