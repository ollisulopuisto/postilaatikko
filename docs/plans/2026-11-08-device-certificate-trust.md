# Device certificate trust store

## Goal

Keep desktop TLS trust-on-first-use exceptions on the device by moving the `certificate.Store` table to `device.db`. Preserve current host-scoped trust semantics and existing user decisions during upgrade.

## Scope and safety

- Create the same `trusted_certificates` schema in `device.db` that service migration v48 currently maintains.
- Add device import marker v4. Copy legacy rows with `INSERT ... ON CONFLICT DO NOTHING`; retain the source table and rows in `aerion.db`.
- Point desktop certificate trust consumers at `device.db`. The helper's scheduled-send certificate store remains service-owned because its transport belongs to the service lifecycle.
- Leave contacts/CardDAV, S/MIME and PGP ownership unchanged.

## Verification

- Test device schema initialization, copy-only import, preservation of preexisting device decisions, repeat initialization, and upgrade from marker v3.
- Run `go test ./internal/appstate ./internal/certificate`, build the desktop and helper, then run `git diff --check`.
