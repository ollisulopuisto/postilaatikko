# GLOBAL-002 recovery checkpoint

Base: fetched `origin/main` at `1817a78` (2026-10-29). New isolated branch `codex/GLOBAL-002-online-counts-resume`, worktree `/private/tmp/postilaatikko-global002-resume`. Existing `codex/GLOBAL-002-online-counts` at `e9f9be5` is preserved. The old worktree is missing: its uncommitted non-Gmail thread-count/header changes were not recovered.

Reapplied committed count jobs/UI and IMAP decreasing-UID continuation from `e9f9be5`, preserving current main's Gmail shared quota budget, thread-list implementation, saved-search virtual-folder query, cached first paint and cache refresh timestamp. Only one `Threads` implementation remains.

Verification at the requested durable WIP checkpoint: `go test ./internal/gmail ./internal/sync ./app` passes. This is not review-ready: saved-query online counts are still being reconciled; frontend checks, broader continuation regressions and full Go suite remain.

Remaining whole-task gates: exact non-Gmail conversation counts/whole-thread criteria, completed-count reuse/invalidation, large-mailbox quota measurements and native/live Gmail/IMAP testing. Unsupported count providers report incomplete. No main merge, release or DMG is claimed.
