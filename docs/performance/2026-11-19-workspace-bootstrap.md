# PERF-SC-003: one-request workspace bootstrap snapshot

2026-11-19 — replaces the desktop client's startup request waterfall with a
single mailbox round trip, plus separate capability discovery.

## Problem

Before first paint, the desktop client issued six separate API calls:
capabilities, accounts, folders, views, unread counts, and the first
conversation page (`startup_waterfall`). On localhost the six requests are
cheap, but on any link with latency each one pays a full round trip. At a
40 ms RTT, startup cost ~300 ms of pure waiting.

## Change

New endpoint `POST /v1/workspace/bootstrap` (read-only, capability
`mail.read`) returns the mailbox sections in one response (capability discovery stays separate):

- `accounts`, `folders`, `views`, `unreadCounts` — always present (empty
  lists/maps when empty);
- `initialPage` — the first conversation page for the requested scope,
  omitted when the request asks for none;
- `errors` — per-section partial failures. Each section runs independently;
  a folder whose provider is unreachable fails that section without failing
  the snapshot. Sections with no error omit their key.

Coherence: sections are fetched independently, so the snapshot carries **no
cross-section revision coherence** (a conversation listed in `initialPage`
may have moved folders by the time `unreadCounts` was computed). Clients
needing strict coherence keep the existing version-token flow. This matches
the pre-change waterfall, which had the same property across six requests.

Body caps and timeouts match the routes the snapshot replaces (the
conversation-page section reuses the exact `executeConversationList` logic
from `POST /v1/conversations`). The route is classified read-only and is
eligible for PERF-SC-002 coalescing.

The `automation/client` grows `BootstrapWorkspace` plus `IsHTTPNotFound`: an
older helper without the route answers a bare 404, and callers fall back to
the six individual reads. The client side of that fallback lives in the
desktop connector change that adopts the snapshot; this PR ships the server
route, the client method, and the detection helper.

## Measurement

`postilaatikko-apibench`, 20k-message fixture, 15 iterations, same machine,
coalescing already in place (scenario executes alone, so flights never
merge):

| scenario | requests/iteration | localhost p50 | 40 ms RTT p50 |
| --- | --- | --- | --- |
| `startup_waterfall` | 6 | 57.2 ms | 299.1 ms |
| `startup_bootstrap` | 2 | 55.8 ms | **136.5 ms** |

Equivalent synthetic startup API work is 2.2x faster on a 40 ms link and similar on
localhost, where the conversation-page query dominates either way. Both scenarios
read capabilities and the same account folder scope. Desktop adoption has not
landed, so this is not measured desktop first-paint performance. The earlier
six-to-one 3.1x claim was withdrawn because it omitted capability discovery
and queried a different folder scope. Corrected reports:
`/tmp/after-003-corrected-local.json`, `/tmp/after-003-corrected-rtt40.json` (bench machine);
baseline: `2026-11-19-performance-baseline.md`.

## Tests

- `v1/workspace_bootstrap_test.go`: full snapshot, empty initial page,
  per-section partial failure isolation, 403 without `mail.read`, no
  coherence field, conversation-page parity with `POST /v1/conversations`.
- `client/bootstrap_test.go`: response parsing, and `IsHTTPNotFound`
  detects a bare-404 helper so fallback stays possible.
- `alwayson/route_classification_test.go`: the new route is classified
  read-only (unclassified routes fail the test).
