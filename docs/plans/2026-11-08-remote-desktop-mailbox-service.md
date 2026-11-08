# Remote desktop mailbox-service connection plan

**Goal:** Let the desktop use the same authenticated mailbox operations against a user-operated service on another computer, so the helper remains the only mailbox database owner.

**Security boundary:** Remote connections require HTTPS with standard certificate and hostname verification. The desktop must never send a bearer token over non-loopback HTTP. Each desktop uses a separate revocable helper API grant; the secret stays in the OS keyring. The helper's explicit Host allowlist remains authoritative. Remote authentication or configuration failures must not switch to local mailbox reads or writes.

## Tasks

1. Add validated service connection configuration to device-owned state: default remains the existing loopback URL; remote URLs must be HTTPS with a host, no userinfo/query/fragment, and no insecure transport override. **Complete.**
2. Update the mailbox connector to construct clients from the configured endpoint. Keep automatic local grant provisioning for loopback. For remote mode, accept a one-time device grant, store it only in the keyring, and validate the grant and required capabilities by querying the remote API, without consulting local service token tables. **Complete.**
3. Expose Wails operations and a settings UI for remote endpoint/token entry, test connection, status, and disconnect. Never display or log a stored token. Avoid exposing write/send capabilities until the user explicitly grants them through the existing capability flow or token. **Complete.**
4. Add focused validation/connector tests and user documentation for issuing and revoking per-device grants on the helper. Update the ownership protocol, changelog, and task ledger. **Complete.**
5. Build affected app/helper packages, run focused tests appropriate to the changed config/client behavior, and check the final diff. Commit the implementation, record verification, then integrate and push `main`. **Complete; remote-only startup remains follow-up.**

Verification: focused `go test ./app` and affected Go builds pass; locale JSON and `git diff --check` pass. `npm run check` could not start because `svelte-check` is not installed. `npm run build` stops in its prebuild script because `frontend/node_modules/@iconify-json/mdi/icons.json` is missing.

## Current boundary

Remote endpoint configuration and authenticated client routing do not yet make the desktop remote-only. Startup still requires a local mailbox database, and unmigrated workflows still construct local stores. The desktop does fail closed for remote configuration, authentication, and availability failures instead of switching to local mailbox data. Remote-only startup and full removal of service-store construction remain separate work.
