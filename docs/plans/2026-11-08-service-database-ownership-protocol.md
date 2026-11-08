# Service Database Ownership Protocol

**Status:** Ownership protocol based on main `6f6197d`, extended by HOSTED-SERVER-025 through HOSTED-SERVER-027. The helper bootstrap handshake and migration-owner transfer are implemented; HOSTED-SERVER-026 routes saved-view definitions and mutations, and HOSTED-SERVER-027 routes automatic filter-rule definitions, through the authenticated helper API. The desktop still opens other service-owned stores, so full database separation remains incomplete.

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

The service database is migrated only by the helper binary. The desktop invokes the sibling helper's versioned `bootstrap` command before opening `aerion.db`, validates the bootstrap protocol, mailbox API version, and schema versions, and does not call `database.Migrate()`. Helper runtime startup, SMTP account transport, API-client administration, and bootstrap use the same ownership lock; account transport no longer migrates independently. Desktop, detached composer, and helper database users hold shared schema locks for their database lifetimes; a pending migration requires an exclusive lock. The helper pauses a running macOS LaunchAgent before a pending desktop-triggered upgrade and restores it afterward. On other platforms it refuses a pending upgrade when the default helper listener is active; manually managed services on custom ports must be stopped by the operator. This transfers schema migration ownership but does not yet remove desktop store construction or direct writes.

## Bootstrap and upgrade protocol

1. Package the helper beside the desktop in development and supported platform builds. Completed by HOSTED-SERVER-024.
2. Add helper bootstrap/health/version behavior and transfer migration ownership. HOSTED-SERVER-025 implements the versioned schema handshake, helper migration, active-process locks, and macOS LaunchAgent stop/restart around pending migrations. The desktop refuses an incompatible helper and does not fall back to desktop migrations.
3. Complete service API ownership of mailbox stores and writes. Until that work lands, the desktop and composer still open `aerion.db` under shared schema locks; only schema migration has one owner.
4. Preserve existing user data in place in `aerion.db`. Add the desktop-only database only for stores proven to be device-local; make any copy/move idempotent and retain a recoverable source until verification succeeds.
5. Fail closed on a newer/unsupported service schema and show a repairable status. Do not silently fall back to opening the service database with desktop stores or run desktop migrations as a fallback.
6. Keep local-only platforms usable by packaging the helper there or by explicitly scoping the always-on service feature and migration owner to supported platforms. Do not claim the server split is complete on a platform that still has desktop-owned service migrations.

## API work required before the split

The service API needs account setup/update and credential handoff, provider folder discovery, saved-view/virtual-folder writes and cache rebuild, filter rule CRUD plus uncertain-job inspection/recovery, triage/Reply Later operations, and draft/send operations with durable provider receipts and sent-message reconciliation. Existing API work already covers mailbox reads, explicit body/attachment fetch, and actions. Each operation should move to the helper's existing shared store/use case; do not add duplicate frontend business logic.

The remaining implementation sequence is:

1. Move persisted mailbox workflow operations (views, filters, triage, drafts) to authenticated service APIs and route the desktop through them. HOSTED-SERVER-026 covers saved-view definitions, mutations, ordering, and cache refresh; HOSTED-SERVER-027 covers filter-rule CRUD/order. Durable filter-job inspection/recovery, triage/Reply Later, and drafts/send remain.
2. Move account/folder setup and provider credential access behind the service boundary, including detached compose.
3. Move device-only stores to a desktop database and remove service-store construction from desktop processes.
4. Verify upgrade, rollback refusal, concurrent launch, stopped service, first run, old-helper upgrade, manual-service recovery, and interruption recovery on every supported platform. Resolve custom-port manual-service detection and exercise the LaunchAgent upgrade lifecycle on a real macOS account.

Helper packaging is implemented by HOSTED-SERVER-024, and the versioned bootstrap/schema-owner cutover is being implemented by HOSTED-SERVER-025. Full server ownership is not complete: the desktop still constructs and writes several service-owned stores, and authenticated APIs for those operations remain required.
