# Mailbox Database Ownership Audit

**Status:** Architecture and dependency audit updated through HOSTED-SERVER-048. Connected desktop virtual-folder unread-count reads now use the authenticated helper API; the desktop still shares the service database for other stores and local fallback.

## Current database and migration ownership

The desktop and helper open the same `aerion.db`, construct overlapping stores, and each can apply migrations:

| Entrypoint | Evidence | Role |
| --- | --- | --- |
| Main desktop startup | `app/app.go:526-544` | Opens the canonical database and calls `Migrate()` before constructing app stores. |
| Headless service startup | `internal/alwayson/sync.go:79-103` | Opens the same data-directory database and calls `Migrate()` before constructing service stores. The process sync lock is stored on the runtime and acquired later, after opening/migration. |
| Helper API-client administration | `cmd/postilaatikko-helper/api_clients.go:56-73` | Opens the same database and also calls `Migrate()` before grant operations. |
| Detached composer | `app/detached_composer.go:156-167` | Opens the same database and constructs account, folder, message, contact, draft, and settings stores without migrating. It depends on the main app having initialized a compatible schema. |

`internal/database/database.go:188-252` reads the current migration version before applying each migration in a separate transaction. SQLite's immediate transaction and busy timeout serialize individual writes, but they do not elect a single process as migration owner across the version-read/apply sequence. The sync lock does not protect migration because service migration precedes sync startup. A second process can therefore observe an old version while the first is upgrading; adding only a generic lock would serialize migrations but would still leave multiple migration owners.

The helper service is not yet the database owner. It is an additional process over a database that remains open in the desktop. The same file currently contains mailbox headers/bodies, account and provider configuration, credentials metadata, folders/views, filters and durable filter jobs, triage/reply-later state, drafts, client grants, and local settings. Contacts, CardDAV sources, certificates and PGP/S/MIME metadata also have desktop stores backed by this database. Some credentials are additionally held in the OS keychain; the in-memory undo stack is not part of this database. The helper requires the account database for sync, filters and authenticated API grants; the desktop still requires it for UI caches and many local operations.

## API boundary already integrated

The authenticated helper API currently mounts:

- account, folder, saved-view, saved-view unread-count, message-header, conversation-list and search reads;
- thread reads, explicit message-body fetch, and attachment list/read;
- idempotent account-qualified archive, mark-read/unread, star/unstar, trash and move actions.

The desktop routes conversation pages, global search, thread reads, on-demand body fetch, explicit mailbox-service connection/grant management, and supported same-account actions through the local API when connected. The desktop reads the service-written body/action state back from its own open database. This keeps the UI compatible with existing stores but is still shared-database coupling.

## Remaining desktop ownership and service gaps

The desktop and helper store construction is visible at `app/app.go:637-689` and `internal/alwayson/sync.go:96-103`. The API's registered routes are at `internal/alwayson/catalog_api.go:66-80`. The API does not yet replace the desktop's direct database/provider use for:

- mail account setup/update, provider folder discovery and account/folder lifecycle;
- virtual-folder and saved-view mutations/cache lifecycle, persistent filter-rule CRUD, and user recovery of uncertain automatic filter jobs;
- draft CRUD, normal immediate send and sent-message reconciliation;
- desktop-local undo journal, preferences, contact/calendar stores, and other device-local settings;
- remaining local list/count/source reads and provider operations that retain explicit local fallback behavior. Connected saved-view unread counts now route to the helper; fallback is limited to local-helper unavailability, while remote authorization/configuration failures fail closed.

Some of these are local presentation/device data and need not move to a remote service. Mailbox authority needs a clear split: authoritative provider/mail state and service workflows must be service-owned; local layout, keychain access and presentation cache may remain desktop-owned. The current single database does not enforce that boundary.

## Dependency order for a safe ownership cutover

1. Define the ownership map at table/store level: service-authoritative mail data, desktop-only data, and client cache. Identify which rows can be rebuilt and which require preservation.
2. Define schema-upgrade coordination before removing either current migration call. It must handle a running older helper, desktop startup, first-run database creation, helper absent/stopped, rollback/schema-too-new behavior, and supported non-macOS builds. The helper binary is bundled in the macOS app; General Settings lifecycle support is currently macOS-only.
3. Complete the API operations required for the selected authoritative data boundary, including account/folder and saved-view/filter/draft operations as appropriate. Keep normal sending and its reconciliation semantics explicit; do not turn a shared database into an accidental API contract.
4. Move the desktop to API-backed authoritative reads/writes and a separately owned/rebuildable presentation cache. Until then, retain the shared SQLite file and current read-after-write behavior.
5. Make the helper the sole schema migrator for service-owned tables, then verify upgrade from each supported prior schema with the desktop and helper already running, stopped, and independently restarted.

## Next implementation slice

Create `HOSTED-SERVER-023` for a table/store ownership map plus schema-upgrade protocol design, then claim a bounded implementation task only after resolving the bootstrap and platform paths. The first code task should not simply remove `App.Startup`'s `Migrate()` call: that would leave first-run, unsupported-platform, or helper-unavailable startup without a safe schema owner. Nor should a file lock be treated as the final ownership transfer while both binaries can still migrate.

## Evidence limits

This audit is based on repository call sites and service route registration, not a runtime trace of every Wails invocation or provider operation. It makes no claim that every desktop store access has been enumerated by individual method. The next task should turn the store-level boundary into a reviewed inventory before data ownership or migrations change.
