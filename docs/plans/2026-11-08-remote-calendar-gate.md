# Remote Calendar feature gate

**Goal:** Keep the optional Calendar extension's Wails bridge safe when remote desktop startup omits local service database and extension stores.

## Scope

- Initialize an inert Calendar bridge during remote startup using device settings only; do not open its local extension store or mailbox database.
- Return an explicit unsupported-mode error for enabled Calendar operations that need local Calendar data or OAuth/provider state.
- Keep pure date detection and best-effort frontend logging safe; preserve the existing local bridge wiring.
- Do not add remote Calendar APIs or change Calendar behavior in local-helper mode.

## Acceptance

- The embedded bridge is non-nil before remote `app:ready` is emitted.
- Remote enabled Calendar operations fail explicitly before local database/store/provider access.
- Remote startup does not construct a local Calendar store or sync/alarms workers.
- The affected app/helper packages build and `git diff --check` passes.
- No tests are added or run for this slice.
