# Spellcheck request lifetime

Based on main `fbb9b32`, inspected 2026-10-28.

The spellcheck client keeps request resolvers in a map. `destroy()` previously
cleared the map without settling the returned promises. Disabling all supported
languages during a check/suggestion could leave the caller waiting indefinitely.
Worker error/message-error handlers only logged, leaving the failed worker and
pending resolver map alive. These are reproduced lifecycle defects, not measured
native process-memory growth.

The fix resolves pending work with the existing empty-result shape, then clears
the map. Error handlers log and destroy the failed worker. Subsequent language
activation can create a fresh worker. Events from an old worker are ignored so
late ready/error notifications cannot affect the replacement. No automatic retry
loop, new network calls, dictionary changes or spelling algorithm changes.

Six mocked-worker regression tests cover destroy/check/suggest, worker error,
message error, normal replies, disabling languages, and stale worker events.
Four of the first five fail against the original implementation, all six pass
after the fix. All 168 frontend tests, type check (zero errors/two existing
warnings), lint, production build and diff check pass. This branch is independent
of the inline-image cache branch, so its test count differs.

No backend code changed. Native WKWebView, real dictionary failure, process RAM,
and interactive compose behavior were not measured here. CI remains a separate
verification step. Returning empty results on termination matches the existing
inactive-worker behavior; a later explicit activation can try again.
