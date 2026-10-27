# GLOBAL-003: live Gmail body and quota repair

User screenshots from v0.3.18-test.4 show headers/snippets arriving, reader cards saying content is unavailable, and an empty virtual view accompanied by Gmail 403 rateLimitExceeded for Total Query Cost / Units per minute per user.

## Confirmed causes and repairs

1. `message.Store.UpsertGmail` omitted `body_fetched` from metadata INSERT. Its historical schema default is 1, so stored header-only rows told the reader no download was needed. The old sync test called body fetch directly, bypassing that reader decision. A new store round-trip assertion failed before the fix; header-only inserts now explicitly set 0. Migration 55 resets affected empty Gmail-native rows to unfetched, preserving nonempty bodies and cryptographic raw content. No source/server mail is deleted or rewritten.
2. Search fetched messages.list + every message's metadata + every matching thread's metadata. It now uses threads.list + one metadata request per thread, followed by one cache projection. No body content is fetched by search. Gmail pages cap at 25 threads; account header backfill/recent refresh caps at 25 messages per pass. The normal reader still fetches full bodies on demand.
3. Separate client instances previously had independent retries and no account-wide quota admission. Engine clients now share a per-account read budget (70 units/sec; FIFO reservations) using current method/format costs. On server rate limits, the shared budget pauses for at least 60 seconds (honoring longer Retry-After), new requests fail promptly with a retryable quota error, and the failing request returns the error once instead of immediate retries. Metadata batches cancel sibling requests at the first failure and preserve its cause, so a quota response does not turn into a long page-wide timeout. Test transports using custom base URLs remain unpaced unless a budget is explicitly injected. Standalone send/helper processes are outside this engine budget; the headroom is intentional and does not guarantee absence of all provider limits.
4. The reader removed a message from its visible conversation after **any** body fetch failure. It now keeps headers, offers Retry, and stops starting downloads after navigation. Cached conversations remain readable if thread metadata refresh is rate-limited or transiently unavailable.
5. A virtual view showed an empty mailbox under its provider error. It now renders a retryable error when no results are available, retains failed-account results only for the same already-loaded view, and avoids a misleading Load more control while the provider is failing. Partial successful-account results remain visible.

## Verification

- Red/green store test proves new header-only rows persist as unfetched and subsequent metadata refresh preserves downloaded bodies.
- Red/green upgrade test repairs empty native rows and preserves body-bearing and non-native rows.
- Cross-account empty-cache integration covers server/virtual query pagination, no duplicate message metadata GETs, no eager body fetch, requested body persistence, cached reader during quota failure, native moves/undo and unconfirmed cross-account copy safety.
- Quota tests cover current costs including full/raw multipliers, shared FIFO reservation/cooldown, cancellation, one response instead of immediate quota retries, and another client's prompt quota response without bypassing cooldown. A 100-item metadata batch stops after the first quota response instead of waiting through its cooldown.
- Full Go suite passed. Affected Gmail/store/database/sync/app race suites passed. Frontend: 162 tests, type check with zero errors and two existing titlebar warnings, targeted ESLint and production build passed. Existing chunk-size advisory remains.
- Real Chromium mounting the reader: first body request returns a simulated quota error; header stays visible; Retry makes the second request and displays its body; no browser exceptions. Temporary fixture removed.

## Release / remaining acceptance

The branch enables GitHub test DMGs with app + Applications shortcut, built after CI validation; no local build is offered as a substitute. Release notes identify cache schema 55. Existing binaries that only support schema 54 cannot open the upgraded profile.

Actual Gmail authorization, sustained mailbox traffic and native WKWebView still require the configured mail device. The fix addresses reproduced code defects; a successful automated test is not a live-device pass. The count/pagination follow-up GLOBAL-002 is parked on its own branch; do not merge its heavy count jobs without reconciling this shared budget. Negative/quoted-phrase exclusion search is recorded as SEARCH-002 and is not included in this repair.

References: [Google Gmail quota costs](https://developers.google.com/workspace/gmail/api/reference/quota), [Gmail thread listing](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.threads/list). New-project limits/costs changed in May 2026; assuming the older 5-unit message GET price would underestimate this workload.
