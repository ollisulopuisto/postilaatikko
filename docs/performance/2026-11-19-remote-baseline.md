# Remote desktop/helper performance baseline (PERF-SC-001)

Date: 2026-11-19. Owner handoff: Postilaatikko Server–Client Performance
Optimization (PERF-SC-001 … PERF-SC-006).

## What this measures

All numbers come from `cmd/postilaatikko-apibench`, which seeds a synthetic
mailbox, opens the production `alwayson.SyncRuntime` + authenticated
`automation.Server` composition, and drives it through the real desktop
connector (`internal/automation/client`). Every scenario reports p50/p95 of
per-iteration wall time, request counts, and transferred bytes; process CPU
and RSS come from `/proc`.

Scope and limits:

- The fixture is synthetic (subjects/snippets, no bodies, no attachments,
  no providers). Results quantify local SQL + HTTP + serialization overhead
  only. Provider (Gmail/IMAP) latency is absent by construction and must not
  be attributed to these numbers.
- Desktop first-paint, WKWebView CPU/RSS, and real-mailbox behavior need the
  owner's Mac; see "macOS follow-ups" below.
- Worker: Linux amd64, 2 cores. Absolute numbers move with hardware; compare
  shapes and scaling, and re-run before/after on one machine for any
  optimization claim.

Instrumentation: the helper now emits one structured timing line per API
request when started with `ETERNOMAIL_API_TIMING=1` (off by default; fields:
request id, method, route pattern, status, duration, response bytes — never
contents, tokens, or query strings; route patterns keep record IDs out of
logs). The desktop connector accepts a custom `http.RoundTripper`, which the
harness uses for request/byte counting and RTT injection.

## Fixture

- 2 accounts, 5 folders each (Inbox/Sent/Archive/Drafts/Trash).
- 20,000 and 50,000 total messages, 90/10 Inbox/Sent, ~4 messages/thread,
  ~14% unread, subjects with a 5% "invoice" minority (every 19th message, so
  invoices land in Sent too) for search.
- Header-only rows (`body_fetched = 0`), matching routine sync.

## Baseline results

Loopback, no injected RTT, p50 (p95), 15 iterations after 2 warmups:

| Scenario | 20k messages | 50k messages | Requests/iter |
| --- | --- | --- | --- |
| startup_waterfall (6 sequential reads) | 56.6 (58.0) ms | 149.0 (153.9) ms | 6 |
| folder_page_first | 55.6 (60.3) ms | 150.9 (157.6) ms | 1 |
| folder_page_deep_offset (offset = half the folder's conversations) | 57.3 (58.4) ms | 160.6 (178.4) ms | 1 |
| folder_count | 30.6 (30.8) ms | 85.0 (89.4) ms | 1 |
| unified_inbox_page | 117.1 (117.8) ms | 308.3 (315.0) ms | 1 |
| conversation_open | 4.3 (4.4) ms | 9.7 (10.1) ms | 1 |
| search_invoice (indexed-header FTS over Sent, ~5% match) | 3.8 (4.4) ms | 7.1 (7.5) ms | 1 |
| concurrent_identical_reads_x8 (wall) | 584.8 (638.0) ms | 1445.4 (1644.2) ms | 8 |

40 ms injected RTT per request, 20k messages:

| Scenario | p50 | Delta vs loopback |
| --- | --- | --- |
| startup_waterfall | 298.4 ms | +242 ms (≈ 6 × 40 ms) |
| folder_page_first | 95.1 ms | +40 ms |
| conversation_open | 44.4 ms | +40 ms |
| search_invoice | 44.0 ms | +40 ms |

Idle (3 s, no traffic): helper CPU 0.0% of one core; RSS 111 MB at the 20k
fixture, 200 MB at 50k (fixture-heavy process; not production-representative).

## Query plans (EXPLAIN QUERY PLAN on the fixture)

`ListConversationsByFolder` (same shape for unified inbox across folders):

```
SEARCH m USING INDEX idx_messages_inbox_category (folder_id=?)
CORRELATED SCALAR SUBQUERY 1  -- gmail_message_refs self probe (indexed)
CORRELATED SCALAR SUBQUERY 2  -- prefer-native-Gmail dedupe (indexed probes)
USE TEMP B-TREE FOR GROUP BY        -- COALESCE(thread_id, id) defeats indexes
USE TEMP B-TREE FOR json_group_array(DISTINCT)
USE TEMP B-TREE FOR ORDER BY        -- latest_date is an aggregate
```

`CountConversations*` repeats the same full-folder grouping to count groups.

## Bottleneck ranking (evidence above)

1. **Full-folder aggregation per page** (`folder_page_first`,
   `unified_inbox_page`, `folder_count`). Every page groups the entire folder
   by `COALESCE(thread_id, id)` with two temp B-trees plus per-row correlated
   dedupe subqueries, then sorts all groups to take 50. Cost grows linearly
   with folder size (56→151 ms at 10k→25k messages per inbox; unified inbox
   117→308 ms) and is paid on *every* page and count. This is the dominant
   remote-mode latency. Deep offsets add only ~5% over the first page, so
   keyset pagination alone (PERF-SC-005) is not the fix; the aggregation
   itself is (maintained conversation summary or cached grouping).
2. **No request coalescing.** 8 concurrent identical reads run the underlying
   query 8 times; wall time ≈ serialized sum (585 ms at 20k). PERF-SC-002's
   premise is confirmed against real code.
3. **Startup waterfall.** 6 sequential round trips: 57 ms loopback, 298 ms at
   40 ms RTT. Each additional RTT multiplies first-usable-inbox latency;
   motivates the PERF-SC-003 bootstrap snapshot.
4. **Idle polling:** none observed in this composition (0.0% CPU at idle);
   provider sync was not running, so idle behavior with live accounts still
   needs the macOS follow-up.
5. Indexed-header FTS search (3.8–7.1 ms) and single-thread open (4–10 ms)
   are already fast at this scale; no action.

## Reproduce

```sh
go run ./cmd/postilaatikko-apibench -messages 20000 -iterations 15 \
  -report /tmp/baseline.json
# High-RTT link simulation:
go run ./cmd/postilaatikko-apibench -messages 20000 -rtt 40ms
# Time the live helper's routes (structured log, off by default):
ETERNOMAIL_API_TIMING=1 postilaatikko-helper serve --api-listen 127.0.0.1:8029
```

On Linux, building the helper packages needs GTK development files
(`libgtk-3-dev`, `libglib2.0-dev`) because `internal/platform` compiles GTK
integration; CI installs them via `scripts/ci_apt_scoped_dns.py`.

## macOS follow-ups (not measured here; needs the owner)

- Cold/warm first-paint and first-usable-inbox latency of the real desktop in
  remote mode; WKWebView CPU/RSS vs helper CPU/RSS
  (`scripts/profile-macos-processes.sh` from PERF-002, plus
  `ETERNOMAIL_PPROF=1` for the Go side).
- The same scenario list against his real mailbox: install the helper with
  `ETERNOMAIL_API_TIMING=1`, use it for a day, and collect the route timing
  lines (they contain only transport metadata).
- Idle CPU/RSS with live Gmail + IMAP accounts syncing.

## Verification in this change

- New Go tests: timing middleware emits exactly one metadata-only line per
  request when enabled (including rejections), nothing when disabled; the
  client `Transport` override is honored.
- `internal/automation/...` tests pass. Native macOS and live-provider
  behavior are unverified by construction (see above).
