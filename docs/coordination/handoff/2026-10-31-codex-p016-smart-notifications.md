# P-016 Smart Notifications handoff

Branch: `codex/P-016-smart-notifications`
Implementation: `aac0365` (`feat: add smart sender notifications`)
Base: `257e7fe`

## Delivered

- Added local notification preferences with a global enable switch and sender/domain rules that either mute a match or explicitly allow notifications.
- Kept the existing notification behavior enabled for users without a saved preference. Unmatched senders continue to notify.
- Sender-specific rules take precedence over domain rules, so an allowed sender can override a muted domain.
- Apply rules to recently synced message headers only. Lookups are capped at 100 headers; database errors or incomplete samples preserve the notification rather than silently dropping potentially allowed mail.
- Added controls in Settings → Inbox, English and Finnish text, settings persistence, and Wails bindings.

## Verification and limits

- `go build ./app` passed.
- `git diff --check`, `gofmt -d` for changed Go files, and English/Finnish locale JSON parsing passed.
- Frontend dependencies were not installed in this worktree, so the Svelte checker/build could not be run. No test suite was run.
- For a burst, the notification is kept if any sampled sender is not muted. If the sample is incomplete, the rules fail open. This deliberately favors avoiding missed mail over fully muting a large batch.
- This is a first slice: it has no per-account, schedule, priority, or quiet-hours rules.
