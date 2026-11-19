# Read request coalescing (PERF-SC-002)

Date: 2026-11-19. Depends on the PERF-SC-001 baseline
(`2026-11-19-remote-baseline.md`).

## Problem

The remote desktop issues identical read-only API calls from several
components at once. The baseline measured 8 concurrent identical folder-page
reads executing the full-folder SQL aggregation 8 times: 585 ms wall at 20k
messages, 1445 ms at 50k, roughly the serialized sum of one page cost.

## Change

`internal/automation/coalesce.go`: a singleflight middleware around the
`/v1/` handler (`internal/automation/server.go`, on by default;
`ServerConfig.DisableReadCoalescing` opts out).

- Eligibility is an allowlist of read-only routes: folder page
  (`POST /v1/conversations`), counts (`POST /v1/conversations/count`),
  searches (`POST /v1/conversations/search`, `POST /v1/search`), thread open
  (`POST /v1/threads/read`), folder/view catalogs (`GET /v1/folders`,
  `/v1/views`, `/v1/accounts`), unread counts (`GET /v1/views/unread-counts`),
  and saved-view cache reads (`GET /v1/views/{id}/cache`). Everything else,
  including every mutation, passes through untouched, so send/move/delete/
  undo idempotency is unchanged.
- Flight identity is SHA-256 over method, path, raw query, body, and the
  Authorization header, so results are never shared across different
  credentials.
- Execution runs on a context detached from any single caller's
  cancellation: a cancelled waiter never cancels work another waiter needs.
  When the last waiter leaves, execution is cancelled (no orphaned queries
  or goroutines). Each waiter keeps its own deadline.
- Bounds: 64 flights in flight, 1 MB keyed request body, 32 MB shared
  response buffer. Overflowing requests execute directly; an oversized
  response fails the flight with 502 rather than truncating or growing
  memory without bound.

## Measured effect

Same worker, same fixture and harness (`postilaatikko-apibench`), before =
the PERF-SC-001 baseline commit, after = this change:

| Scenario | 20k before | 20k after | 50k before | 50k after |
| --- | --- | --- | --- | --- |
| concurrent_identical_reads_x8 (wall p50) | 585 ms | 56 ms | 1445 ms | 146 ms |
| folder_page_first p50 (unchanged path) | 56 ms | 55 ms | 151 ms | 151 ms* |

*single-request scenarios are statistically unchanged; the after run's full
table is in the PR description.

The after wall time equals one folder page execution, the signature of one
shared query. The unit tests assert the stronger property directly: 8
concurrent identical requests execute the handler once (race detector
clean), distinct bodies/tokens stay separate, a cancelled leader does not
cancel shared execution, the last departing waiter cancels execution, and
mutations are never merged.

## Remaining work in this area

- Client-visible cancellation of superseded reads on navigation (folder
  switch, search refinement) needs the Wails call path; server-side
  cancellation semantics are in place for it, but the desktop binding does
  not propagate aborts today. Tracked under PERF-SC-002 follow-ups.
- The single aggregation itself remains the top bottleneck (PERF-SC-005);
  coalescing only removes duplicate executions of it.
