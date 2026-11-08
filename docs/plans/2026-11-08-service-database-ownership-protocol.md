# Service Database Ownership Protocol

**Status:** Design based on main `6f6197d`; no runtime changes. This extends the [database ownership audit](2026-11-08-mailbox-db-ownership-audit.md) with a store-level boundary and the first migration steps.

## Current store map

| Store group | Desktop | Headless service | API status | Target owner |
| --- | --- | --- | --- | --- |
| Accounts, provider configuration and credential access | `accountStore`, `credStore`; account CRUD and credential writes in `app/account.go` | `accountStore` and credential-backed sync | Catalog reads only; no account setup/update API | Service; desktop setup becomes API client and uses the OS keychain only for device-local secrets where appropriate. |
| Folders and provider folder discovery | `folderStore`; direct list, tree, sync and mapping operations | `folderStore`; sync and API reads | Folder catalog read only | Service for provider state; UI may keep a presentation cache. |
| Messages, attachments and sync state | `messageStore`, `attachmentStore`, direct provider fetch fallbacks | Same stores; sync, filter moves, API reads/actions | Headers, conversations, search, thread/body, attachment list/read, flags and supported moves | Service for authoritative provider state; desktop cache must be rebuildable and separate. |
| Saved views and virtual folders | `virtualFolderStore`, cache/read/write methods | `virtualFolderStore` and API view reads | View list only; no CRUD/cache maintenance API | Service for persisted query definitions and indexes; desktop presentation state stays local. |
| Automatic filters and jobs | `filterStore` CRUD and local fallback execution | `mailfilter.Store`, durable `JobStore`, background execution | No rule CRUD or uncertain-job inspection/recovery routes | Service for rules, execution and job state; UI manages through authenticated API. |
| Triage and Reply Later | `triageStore`, `replyLaterStore`, direct UI operations | Same stores | No API routes currently mounted | Service for cross-device mailbox workflow state. |
| Drafts and immediate send | `draftStore`, SMTP/provider send, sent reconciliation | No draft store or normal send worker | No draft/send routes | Service for durable mailbox drafts/send state after API supports receipt, cancellation and reconciliation semantics. |
| Device and security data | settings, app state, content policy, contacts/CardDAV, certificates, S/MIME and PGP stores | Not constructed by `SyncRuntime` | No API routes needed for local-only presentation; auth grants are service DB data | Keep device layout/preferences and OS-specific contact/key UX local. Decide per-store whether provider trust/configuration belongs with service credentials. |
| API grants and helper lifecycle | Desktop keyring credential and connector; local lifecycle controls | SQLite token store and listener | Authenticated localhost API with revocation | Grants and server state stay service-owned; desktop keeps only its client token in the OS keyring. |

The desktop store construction is in `app/app.go:637-689`; the helper's is in `internal/alwayson/sync.go:96-115`. `app/detached_composer.go:156-203` independently opens the canonical database and constructs overlapping account/message/draft/security stores. The table is a current dependency map, not a claim that every Wails call site was enumerated.

## Decision: separate service and desktop databases

Keep the current `aerion.db` as the service-owned authoritative mailbox database during the transition. Move only clearly device-local state to a separate desktop database after its callers are identified. Do not give the desktop a writable handle to the service database as a permanent cache strategy. Service API results may populate a bounded, rebuildable presentation cache owned by the desktop.

The service database must be migrated only by the service binary. The desktop must stop calling `database.Migrate()` for that database and must stop constructing service-owned stores from it. A helper-only `migrate` CLI command is insufficient if the desktop can still open the same database and migrate it too. A database file lock around the current two callers serializes work but does not establish the required owner.

## Bootstrap and upgrade protocol

1. Package a same-version headless helper wherever the desktop is supported, and make it available in development builds. Current packaging bundles it only in the macOS app (`Makefile:61-67`); `make dev`, Linux, Flatpak and Windows targets do not bundle it. Until this matrix is resolved, the service-owned database cannot have one migrator across all desktop start paths.
2. Add helper bootstrap/health/version behavior. On first launch, the desktop starts or connects to the helper before it opens service-owned stores. The helper creates or upgrades `aerion.db`, then reports a compatible schema and API version before the UI proceeds.
3. During app/helper upgrades, coordinate the service lifecycle: do not migrate while an older helper is serving requests. Stop or drain the older process, upgrade through the new helper, apply migrations, then restart and reconnect. A stopped service remains stopped unless the user had it running before the upgrade.
4. Preserve existing user data in place in `aerion.db`. Add the desktop-only database only for stores proven to be device-local; make any copy/move idempotent and retain a recoverable source until verification succeeds.
5. Fail closed on a newer/unsupported service schema and show a repairable status. Do not silently fall back to opening the service database with desktop stores or run desktop migrations as a fallback.
6. Keep local-only platforms usable by packaging the helper there or by explicitly scoping the always-on service feature and migration owner to supported platforms. Do not claim the server split is complete on a platform that still has desktop-owned service migrations.

## API work required before the split

The service API needs account setup/update and credential handoff, provider folder discovery, saved-view/virtual-folder writes and cache rebuild, filter rule CRUD plus uncertain-job inspection/recovery, triage/Reply Later operations, and draft/send operations with durable provider receipts and sent-message reconciliation. Existing API work already covers mailbox reads, explicit body/attachment fetch, and actions. Each operation should move to the helper's existing shared store/use case; do not add duplicate frontend business logic.

The implementation sequence is:

1. Package/launch the helper as a local service in development and supported desktop builds; establish a versioned bootstrap handshake while retaining current data and behavior.
2. Move persisted mailbox workflow operations (views, filters, triage, drafts) to authenticated service APIs and route the desktop through them.
3. Move account/folder setup and provider credential access behind the service boundary, including detached compose.
4. Move device-only stores to a desktop database and remove service-store construction from desktop processes.
5. Remove desktop migrations for `aerion.db`; verify upgrade, rollback refusal, concurrent launch, stopped service, first run, and recovery from interruption on every supported platform.

The first implementation task should be step 1: helper packaging and a versioned bootstrap handshake. It must include dev mode and supported non-macOS build targets or explicitly narrow the service-supported matrix with product approval before changing startup behavior.
