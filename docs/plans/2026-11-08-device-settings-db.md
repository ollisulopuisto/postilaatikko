# Device Settings Database

## Goal

Move desktop-only preference and image-loading policy state out of the helper-owned mailbox database into `device.db`.

## Scope

- Initialize the minimal `settings` and `image_allowlist` tables in the device database without running mailbox migrations.
- Copy legacy rows once using transactional, insert-if-absent imports; retain source rows in `aerion.db` for rollback and recovery.
- Construct desktop settings and image-allowlist stores on `device.db`; leave provider-owned settings and other stores untouched.
- Add focused import, restart, and store-routing regression coverage and update ownership documentation/changelog.

## Acceptance

- Existing preferences and image allowlist entries remain available after upgrade.
- Re-running initialization is safe and does not overwrite device-local edits with old service-database values.
- Desktop settings and image policy writes target `device.db`; service DB legacy tables remain untouched.
- Focused Go tests, relevant build, and `git diff --check` pass.

## Verification

- `go test ./internal/appstate ./internal/settings ./app` (focused selectors as appropriate)
- `go build ./app ./cmd/postilaatikko-helper`
- `git diff --check`

## Risks

- `settings` contains many UI preferences; copy-only import is required to keep downgrade recovery possible.
- `ImageAllowlistStore` is local display policy and must stay device-local; other message content policy remains service data until separately reviewed.
