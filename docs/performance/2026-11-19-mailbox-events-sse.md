# PERF-SC-004: server-sent events for mailbox changes

2026-11-19 — authenticated, bounded server-to-client change notifications,
replacing UI polling with push.

## Problem

Desktop clients learned about mailbox changes only by re-issuing read
requests. With PERF-SC-001..003 in place (cheap coalesced reads, one-request
bootstrap), the remaining cost is idle polling itself: every connected
client re-reads folders and counts on a timer whether or not anything
changed.

## Change

New `GET /v1/events` server-sent events stream (capability `mail.read`):

- **Invalidation metadata only.** Events are `{sequence, type, accountId?,
  folderId?, at}` — never message contents, credentials, or paths. Types:
  `mailbox.changed`, `folders.changed`, `views.changed`, `drafts.changed`,
  `triage.changed`, and the `mailbox.resync` sentinel.
- **Monotonic sequence + gap detection.** Sequences are strictly increasing
  per helper lifetime. Reconnects send `Last-Event-ID`; the server replays
  its retained ring (512 events) and answers an unrecoverable gap with
  `mailbox.resync`, on which the client re-reads state via
  `/v1/workspace/bootstrap` (PERF-SC-003).
- **Burst coalescing.** Repeated changes to the same type/account/folder
  within 500 ms publish once; a 500-message flag sync yields one event per
  folder.
- **Bounded resources.** 16 subscribers max (503 retryable beyond), 512-event
  replay ring, 64-event per-subscriber queue whose overflow degrades to a
  resync marker instead of memory growth.
- **Revocation revalidation.** Every 30 s the stream re-checks the grant
  against the same token store as the auth middleware; a revoked client's
  stream ends and its reconnect gets 401. A 25 s heartbeat keeps the stream
  alive through proxies. Stream headers flush at connect, so clients see the
  subscription succeed immediately.
- **Prompt shutdown.** Closing the runtime closes the broker, which closes
  every subscriber channel, ending streams at once.
- **No new provider connections.** Events are fan-out from state the helper
  already owns (sync engine, action executor, stores): one provider
  connection per account regardless of desktop client count.

Published sources: provider sync completion (per folder), the engine's
new-message hook, `/v1/actions` execution (source and destination folders),
action undo (per affected folder), folder catalog sync, saved-view
create/update/delete/reorder/cache refresh, draft save/delete, triage and
reply-later mutations. Scheduled-send state is not on the /v1 automation
surface (it lives in the desktop IPC layer), so it publishes nothing here;
its route family can subscribe its mutations later without protocol changes.

The client grows `SubscribeEvents(ctx, lastEventID, onEvent)`: parses the
SSE stream, sends `Last-Event-ID`, ignores heartbeat comments, surfaces HTTP
errors through the same envelope as other calls. Reconnect policy belongs to
the caller: on stream end, reconnect with the last sequence seen; on
`mailbox.resync`, re-read via bootstrap.

Periodic sync stays the provider recovery mechanism; the event stream only
removes *UI* polling.

## Tests

- `internal/mailboxevents/broker_test.go` (race-clean): monotonic sequence,
  burst coalescing window, replay and gap detection, slow-subscriber
  resync-instead-of-growth, subscriber limit and idempotent cancel, live
  delivery and close semantics.
- `internal/automation/v1/events_sse_test.go`: live delivery with SSE
  framing, two subscribers converging on the same event, resync marker
  before replay, malformed `Last-Event-ID` → 400, saturated source → 503,
  stream ends on revocation and on broker close, capability denial → 403.
- `internal/automation/client/events_test.go`: stream parsing (heartbeat,
  multi-line safety, resync), `Last-Event-ID` header, HTTP error surfacing,
  context cancellation.
- `alwayson/route_classification_test.go`: `GET /v1/events` is classified
  non-coalescible (coalescing a stream would buffer it).

## Idle request effect

The measurable win lands with desktop adoption (follow-up): a client that
today polls unread counts and the visible folder every 30 s issues 2
requests/30 s idle; on the stream it issues zero — one long-lived GET
replaces the poll loop, and requests happen only in response to real
change events. apibench has no mutation source to drive a meaningful
events scenario, so the harness comparison is deferred to the desktop
connector change.

Baseline: `2026-11-19-performance-baseline.md`; bootstrap:
`2026-11-19-workspace-bootstrap.md`.

## Review corrections

- Burst coalescing emits a leading invalidation and a bounded trailing invalidation for suppressed writes. One timer and at most 1024 dirty keys are retained; Close stops the timer.
- Numeric cursors do not encode broker lifetimes. Every nonzero-cursor reconnect now conservatively requests a full resync before replay, including an empty restarted broker and overlapping sequence values. This trades extra reconnect reads for correctness without introducing a second cursor protocol.
- SSE renews a 30-second write deadline before each write and uses ResponseController.Flush through middleware wrappers. The ordinary HTTP request timeout is disabled only on a private client copy for streaming; cancellation remains context-owned.
- Grant lookup errors and missing authenticated client IDs end streams rather than leaving them unverifiable.
