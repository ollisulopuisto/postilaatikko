# Remote bootstrap seam

**Goal:** Prepare safe primitives for remote-first desktop startup without changing the current startup mode yet.

## Tasks

1. Split device-owned table/schema initialization from copy-only imports out of the legacy mailbox database. Keep the existing `InitializeDeviceDatabase` behavior as a compatibility wrapper for local startup.
2. Permit device-only initialization with no mailbox database without recording one-time legacy import markers. When a mailbox database later becomes available, imports must still copy rows idempotently and leave source rows intact.
3. Allow the remote mailbox connector to be constructed and its per-device keyring credential/client validated without a local database. Local-helper mode must still require local token-table validation and preserve automatic grant provisioning.
4. Add regression coverage for remote connector construction with no service DB, local grant paths, device-only boot then later legacy import, and repeated idempotent initialization.
5. Do not switch `Preflight` to remote-only startup in this task; that depends on the later minimal app-startup/service-store boundary.

## Acceptance

- Existing callers of `InitializeDeviceDatabase(deviceDB, legacyDB)` retain copy-only import behavior and marker semantics.
- A nil legacy DB creates/upgrades device-owned schema only, and leaves all migration markers for legacy copy operations unapplied.
- A later call with a real legacy DB performs the pending imports once, without deleting legacy rows or overwriting device values.
- Remote connector setup does not require or query the local service DB; local mode still validates grants and auto-provisions exactly as before.
- Focused tests, affected package builds, and `git diff --check` pass.
