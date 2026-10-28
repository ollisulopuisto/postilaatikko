# Inline-image session cache

Based on main `fbb9b32`, inspected 2026-10-28.

## Finding

`frontend/src/lib/stores/inlineAttachmentCache.ts` retained every fetched inline
attachment data URL until an account/cache reset or process exit. Opening more
messages could therefore retain more image strings indefinitely. This is an
unbounded cache, not proof of a leak in the native image renderer.

`EmailBody.svelte` reads this cache first, then calls `GetInlineAttachments` on a
miss. It keeps the returned data in its own state while open. SQLite owns durable
attachment content. Eviction therefore releases this cache's reference without
revoking open images or deleting mail. Existing generation checks prevent stale
requests from repopulating a cleared cache and are unchanged.

## Change

Bound the cache to 128 messages and 32 MiB of accounted UTF-16 string payloads,
including message/content IDs, with least-recently-used eviction on reads/writes.
An entry larger than the payload budget is displayed normally by the reader but
not retained in the session cache. These are conservative initial cache limits,
not measured optimal settings. They trade a repeat local-storage/bridge read for
bounded retention. Existing callers do not mutate cached maps.

The string budget is not a total process-RAM limit: JS object overhead, decoded
images, iframe state, and open readers are not included.

## Verification

Seven regression tests cover retrieval/reset generation, read recency, message
and string budgets, oversized entries, replacement accounting, clear accounting,
and key sizes. Five tests failed against the original cache; all pass after the
change. A synthetic series of twelve distinct messages with 2,097,152-character
image values retained twelve entries before and seven after (about 48 MiB versus
28 MiB in conservative two-byte string accounting, excluding small keys). This
is deterministic cache-retention characterization, not observed native RAM.

- All 169 frontend tests pass.
- Frontend type check: zero errors; two existing warnings.
- Frontend lint and production build pass. Build reports existing large chunks.
- `git diff --check` passes.

No Go/backend code changed. No native macOS GUI, real mailbox, process-RAM or
startup timing was measured. Full CI remains a separate check on the PR.
