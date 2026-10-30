# UX-026 Reply Later queue recovery and API adapter

Base: `origin/main` at `2da8001` (PR #72 integrated).
Branch: `codex/UX-026-queue-api-recovery`.

## Delivered in this follow-up

- `replylater.Store.List` now reports whether each durable queue reference has
  a matching locally indexed message/thread. This is based on the account and
  stable conversation ID, not whether a row happened to be in the currently
  loaded page.
- The Reply Later view renders unindexed references with the affected account,
  stable reference, and a remove action. Removal uses the existing undo-aware
  queue operation. The English and Finnish strings are included.
- The queue store refreshes this index status when one of the referenced
  accounts finishes syncing, so a recoverable item stops appearing as stale
  after its messages are indexed.
- `internal/automation/v1.ReplyLaterAdapter` provides capability-checked list,
  add, and remove operations. Reads require `mail.read`; changes require
  `mail.write`. Mutations delegate to the app's normal durable undo/event path.

## Boundary and remaining integration

This package is transport-independent and opens no listener. UX-020 must bind
it to an authenticated caller and a real capability authorizer; UX-021 provides
the read API foundation. Do not expose it through an unauthenticated route.
The existing Wails/UI queue remains independent of the external automation
transport.

## Verification

`gofmt` and `git diff --check` completed. Tests, frontend checks, and builds
were not run in this pass.
