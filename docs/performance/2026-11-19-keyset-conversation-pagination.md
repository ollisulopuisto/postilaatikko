# PERF-SC-005: keyset conversation pagination

Date: 2026-11-19. Branch: agent/PERF-SC-005-query-opt.

## Problem

`POST /v1/conversations` paginated by numeric offset. Every page ran one
query that grouped and sorted **every message in the selected folders**:

- `GROUP BY COALESCE(m.thread_id, m.id), a.id` over the full folder set,
- heavy aggregates per group (`GROUP_CONCAT` of all message ids,
  `json_group_array(DISTINCT ...)` of participants),
- `ORDER BY latest_date DESC` + `LIMIT/OFFSET`.

`EXPLAIN QUERY PLAN` (20k messages / 5000 threads, synthetic apibench-shaped
fixture, this Linux worker) showed `SEARCH m USING INDEX
idx_messages_inbox_category (folder_id=?)` plus `USE TEMP B-TREE FOR GROUP
BY`, `USE TEMP B-TREE FOR group_concat(ORDER BY)`, `USE TEMP B-TREE FOR
json_group_array(DISTINCT)`, and `USE TEMP B-TREE FOR ORDER BY`. Measured:

| page | offset path |
| --- | --- |
| offset 0 | 114 ms |
| offset 1250 | 119 ms |
| offset 2500 | 120 ms |
| offset 3750 | 121 ms |

Every page costs O(folder size) no matter where it starts; deeper folders
scale linearly in total. Numbers from the env-gated harness:
`PERF_PROFILE=1 go test ./internal/message/ -run QueryProfile -v`
(worker box, not the macOS target; relative costs are the point).

## Design

Two-phase keyset pagination, newest-first, unfiltered lists only:

1. **Phase 1 — page keys.** Per folder, walk messages newest-first:
   `SELECT COALESCE(thread_id, id), date FROM messages WHERE folder_id = ?
   AND date <= ? <visibility clauses> ORDER BY date DESC, thread ASC`.
   The new covering index `idx_messages_folder_date_conv(folder_id, date
   DESC, COALESCE(thread_id, id))` turns this into an index seek starting at
   the cursor position. The first occurrence of a thread in newest-first
   order is its latest message, i.e. its sort key; Go deduplicates threads,
   merges folders with the offset path's tie-breakers (`date DESC, account_id
   ASC, thread ASC`), and stops after `limit` keys plus one.
2. **Phase 2 — page aggregates.** The original aggregate query, restricted to
   the page's conversations via `idx_messages_account_conv(account_id,
   COALESCE(thread_id, id))`, with identical Gmail duplicate suppression and
   visibility clauses. Temp B-trees remain but touch at most a page of rows.

The cursor is an opaque `k1:`-prefixed base64url JSON tuple
`(latest_date, account_id, thread, order)`.

Measured on the same fixture:

| page | keyset path |
| --- | --- |
| page 0 | 4.0 ms |
| page 24 (rows 1200+) | 3.1 ms |
| page 49 (rows 2450+) | 3.0 ms |
| page 74 (rows 3700+) | 3.6 ms |

Cost no longer scales with skipped rows or folder size: ~30x faster at this
fixture size, and flat with depth, satisfying the acceptance criterion.

## Index cost

Migration 84 adds both indexes. Seeding 20k messages
(`PERF_PROFILE=1 go test ./internal/message/ -run IndexWriteCost -v`):

- without the indexes: 1.12 s
- with the indexes: 1.23 s (+9.7%, ~5.5 µs/message)

Bulk sync of a 20k-message mailbox pays ~110 ms once; steady-state single-row
writes pay microseconds. Considered not material.

## Consistency contract

- Messages inserted or moved newer than the cursor between pages belong to an
  already-served region and do not shift or duplicate later pages; they
  appear on the next first-page refresh.
- Conversations removed between pages are skipped.
- A conversation whose latest message moves across the cursor between pages
  can reappear once, matching the offset path under the same mutation.

`TestKeysetMatchesOffsetPathNewest` walks all pages of a two-account fixture
and asserts identity with the offset path (order and per-conversation
content). `TestKeysetSkipsConversationsInsertedBetweenPages` covers the
insert-between-pages case.

## Scope decisions

Keyset serves: folder scope, all_inboxes, and unified folders backed by one
physical folder list (drafts, sent, trash, starred, spam) — when the list is
unfiltered and newest-first.

Offset pagination remains for:

- legacy numeric cursors (existing clients during transition),
- filtered lists (filters are HAVING clauses over full-folder aggregates),
- `oldest` ordering (ranking by latest date ascending needs each thread's
  newest message before ranking — the full-folder grouping the keyset path
  exists to avoid),
- unified archive (merges physical and virtual listings per page) and the
  read-later/reply-later queues (own indexed store paths).

The handler picks the keyset path only for an empty or `k1:`-prefixed cursor,
so offset and keyset never mix within one pagination session. Operation sets
that do not implement `ConversationListKeysetOperations` fall back to offset.

## Revision-keyed count caching

Evaluated and deferred: `/v1/conversations/count` already reuses the
queue-driven paths for read-later scopes and the count query is one indexed
`COUNT(DISTINCT ...)` per request. The coalescing layer from PERF-SC-002
already deduplicates concurrent identical count reads. A revision-keyed cache
adds invalidation surface without a measured hot path; revisit with
production traces.

## Files

- `internal/message/keyset.go` — cursor codec, two-phase store pagination.
- `internal/message/keyset_test.go` — equivalence, mutation, malformed cursor,
  archive-variant tests.
- `internal/message/store.go` — shared query builder/visibility clauses used
  by the offset path, the keyset page loader, and plan profiling.
- `internal/message/query_profile_test.go` — env-gated EXPLAIN/timing harness
  (offset vs keyset, index write cost).
- `internal/database/migrations.go` — migration 84 (two indexes).
- `internal/automation/v1/conversation_list.go` — handler dispatch and
  `ConversationListKeysetOperations`.
- `internal/alwayson/catalog_api.go` — keyset operation wrappers.
