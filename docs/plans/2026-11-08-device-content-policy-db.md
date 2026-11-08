# Device Folder Content Policy Database

## Goal

Move explicit folder body-prefetch overrides to the device-owned database because they govern local presentation and prefetch behavior, not provider mailbox state.

## Scope

- Initialize the folder content policy table in `device.db` without service migrations or cross-database foreign keys.
- Import legacy per-folder overrides once, preserving source rows and existing device-local values.
- Route desktop policy reads/writes to the device database and clear overrides when an account is removed.
- Preserve the automatic defaults derived from Inbox, Starred, and sidebar pins.

## Acceptance

- Existing explicit enable/disable choices survive first startup and rollback remains recoverable.
- Reinitialization does not overwrite later device-side edits.
- Account removal purges local overrides; service-side source rows remain for recovery.
- Focused migration/store/account lifecycle tests, build, and `git diff --check` pass.

## Verification

- `go test ./internal/appstate ./internal/folder ./app` (focused selectors as appropriate)
- `go build ./app ./cmd/postilaatikko-helper`
- `git diff --check`

## Risks

- The source table currently cascades on account/folder deletion. The device-owned copy has no cross-database foreign key, so account removal must explicitly purge those rows.
