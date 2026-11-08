# Remote-first desktop startup and presentation-cache boundary

**Goal:** Let the desktop start as a client of a separately running mailbox helper, including one on another computer, while keeping the frontend responsive through bounded API reads and rebuildable device-local presentation state.

**Scope:** Design only. Do not change runtime behavior or move data in this task.

## Evidence

- `app.Preflight` currently bootstraps the mailbox database and credentials before Wails starts; `app.Startup` assumes `a.db`, `a.deviceDB`, and `a.credStore` are initialized.
- `Startup` constructs service-backed stores over the local mailbox DB for accounts, folders, messages, triage, saved views, filters, attachments, drafts, contacts, CardDAV, S/MIME, PGP, and provider transports. It also starts local maintenance loops and initializes mailbox operations.
- The remote connector authenticates using a per-device keyring grant and remote capability discovery, but connector construction still receives the local DB. The desktop startup path still opens/migrates the local mailbox DB before it can connect.
- Remote desktop settings and app/device state already have a device-owned database. The helper owns warmed virtual-folder membership caches and exposes authenticated cache pages; connected desktop reads also obtain saved-view unread counts from the helper.
- Current API-backed behavior is mixed with local reads and writes. Several of those local paths are legitimate device state or temporary fallback, while authoritative mail data and service workflows need a clear owner.

## Store and operation ownership inventory

| Area | Current startup / call-site evidence | Current class | Remote-first requirement |
| --- | --- | --- | --- |
| Accounts and provider credentials | `Preflight` creates a credential store over the mailbox DB; `Startup` creates `accountStore`; account methods route through `account.manage` when authorized. | Service-authoritative, with some credential configuration still initialized locally | Fetch public account metadata from helper; keep all secrets on helper; no client credential-store dependency for remote mode. |
| Folders and subscriptions | `Startup` creates `folderStore`; list/sync/subscription methods have helper routes with local fallback. | Service-authoritative | Route catalog and lifecycle operations to helper; local path only in local-helper mode. |
| Message headers, thread bodies, attachments, search | Startup constructs `messageStore`, `attachmentStore`, sync engine and FTS; API already exposes bounded pages, search, thread/body and attachment operations. | Service-authoritative; presentation/query caches are mixed | Make first paint bounded and API-backed; body and attachment bytes remain explicitly fetched. FTS/provider engines must not be constructed for remote clients. |
| Saved views | `virtualFolderStore` is constructed locally; list/mutations, cache reads/search, and unread counts are routed to helper when connected. Helper warms membership cache on startup and periodically. | Service-authoritative definitions and helper-owned result cache; local fallback remains | Load helper-cached view pages first with freshness; no desktop query against service tables in remote mode. |
| Triage, queues, filters, drafts/send | `Startup` constructs `triageStore`, `replyLaterStore`, `filterStore`, `draftStore`, filter router and pending-draft work; related API coverage is partial and send/draft operations depend on current shared stores. | Service-authoritative workflows, API coverage incomplete | Keep each required operation behind a capability-checked service method; do not enable a UI path with hidden local writes. Detached composers must use the same configured endpoint. |
| Undo, app state, layout, preferences, window state | `appstate` and settings use `device.db`; cross-process Undo remains desktop-local. | Device-owned state, with undo operation references needing service-safe identity | Retain local UI state; define how remote mutations produce undo records and what happens if their operation is no longer available. |
| Contacts, CardDAV, S/MIME, PGP | `Startup` constructs `contactStore`, `carddavStore`, `smimeStore`, `pgpStore` and provider crypto; paths still reference mailbox DB/credential store. | Separate product features; currently service DB-coupled | Either migrate their device/user data and API dependencies or disable these features for remote-only mode until explicitly supported. Never silently create empty shadow stores that appear authoritative. |
| Device policy and trust | Content policy, image allowlist, certificate trust and settings are initialized from `device.db`; legacy import currently receives the mailbox DB. | Device-owned | Split device schema initialization from copy-only legacy imports; remote mode can open existing/new device state without any service DB handle. |

## Startup and first-paint sequence

1. Resolve platform paths and create only the device directories.
2. Open and migrate `device.db`; load the configured endpoint and non-secret device settings. If no remote endpoint is configured, retain the existing local-helper flow.
3. In remote mode, construct a remote connector from endpoint + keyring grant without opening the service DB, running helper bootstrap, taking the local service schema lock, or initializing local service stores. Validate endpoint/TLS and fetch capabilities before exposing operations.
4. Fetch a bounded account/folder/view catalog. Use the helper-owned virtual-folder cache page and unread counts for an immediate saved-view first paint. Show cached data with freshness and an explicit stale/offline state when allowed; do not claim freshness until server confirmation.
5. Fetch the visible conversation page from the helper, then hydrate thread metadata; fetch full message body or attachments only on user request. Apply newer helper events or an explicit refresh without reloading unrelated folders.
6. Construct optional feature adapters only after negotiated capabilities and endpoint mode are known. Unsupported remote features remain visibly unavailable and cannot fall through to local providers.

The device presentation cache does not exist as a coherent replaceable layer today. Define its schema only after the store inventory identifies which rows are safe to rebuild. Never put bearer credentials or authoritative drafts in that cache. Preserve device-authored layout/queue/undo state across cache rebuilds.

## Failure and reconnect behavior

| Condition | Local-helper mode | Remote-helper mode |
| --- | --- | --- |
| Service absent | Keep current bounded local-helper unavailable fallback where supported | Show disconnected state; serve only explicitly marked stale presentation cache if available; no local mailbox DB fallback |
| Token missing/revoked or capability denied | Preserve existing consent/re-authorize flow | Block protected operation; retain endpoint; never try local grant tables or local mailbox data |
| TLS/configuration/version mismatch | Surface local helper/bootstrap issue | Fail closed before service operations; keep recoverable device configuration and actionable status |
| Network interruption during read | Existing local fallback may be used where the local helper is configured and unavailable | Reuse only cache with freshness metadata; mark incomplete/stale; explicit retry |
| Ambiguous write/send timeout | Existing durable idempotency and operation receipts govern retry | Reuse the same operation ID/idempotency key; show uncertain state where status cannot be confirmed; never replay with a new key automatically |
| Endpoint switch | Local endpoint remains default unless user changes it | Clear endpoint-scoped in-memory clients and caches; retain user-authored device state; require authentication to the new endpoint |

## Dependency-ordered implementation slices

1. **Remote bootstrap seam:** initialize device DB independently; read endpoint before helper bootstrap; make remote connector/client constructible without a mailbox DB. Test remote preflight with no local service DB and local default compatibility.
2. **Remote core workspace:** initialize only the app/window/local-state services plus helper API adapters; route account/folder/catalog, conversation/search, virtual-folder cache/count, thread/body and explicit action paths without constructing service stores or provider pools.
3. **Remote drafts and compose:** finish helper-backed draft list/read/write/send and detached composer endpoint propagation; test revision conflicts, ambiguous idempotent retries, and Sent reconciliation.
4. **Remote background features:** remove desktop helper-owned sync/filter/snooze/queue loops; subscribe to bounded server change events and refresh only affected visible data.
5. **Feature gates and remaining stores:** move or explicitly gate contacts, CardDAV, crypto, contact OAuth and other non-core integrations. Remove mailbox DB schema ownership/opens from the desktop only after migration/bootstrap compatibility and downgrade/recovery paths are proven.
6. **Presentation cache:** add versioned local cache for the minimum catalogs/pages justified by measured first-paint needs; rebuild safely from helper snapshots and maintain event-based invalidation. The helper remains the authoritative virtual-folder cache owner.

## Acceptance for the remote-only startup implementation

- An integration test creates a device DB and remote API fixture but no mailbox DB/helper binary, starts the app in remote mode, connects successfully and loads account/folder/saved-view catalog plus a cached conversation page.
- Startup does not create or migrate the mailbox DB, acquire its schema lock, initialize a provider transport, or execute local mailbox SQL in remote mode.
- Remote auth/configuration/capability errors fail closed, and local default startup still bootstraps the helper and follows existing fallback behavior.
- Disconnect/revocation/offline UI states distinguish empty from unavailable/stale results; no writes are silently queued or sent to another endpoint.
- Body and attachments remain on demand; first paint has an explicit bounded request budget.
- Capability-gated optional integrations cannot dereference nil service stores or bypass the selected endpoint.

## Design decisions to make

1. Define startup modes from explicit device configuration: local helper (default), configured remote helper, and disconnected/unconfigured state. A remote auth/configuration failure must remain visible and must never activate local mailbox fallback.
2. Separate device bootstrap from mailbox-service bootstrap. Device preferences, endpoint, credentials and recoverable UI state must initialize without opening or migrating the service DB.
3. Define the minimal client state needed for a fast first paint. Prefer bounded helper API responses and a replaceable local presentation cache for account/folder/view catalogs and visible header pages. Specify cache versioning, freshness, invalidation, offline behavior, and how cache rows are rebuilt without losing user-authored data.
4. Inventory each `App` store/operation as service-authoritative, device-owned, rebuildable cache, or unresolved. For unresolved paths, specify whether the feature is disabled, API-backed, or requires the local helper. Include detached compose and external `mailto:` startup paths.
5. Define reconnect, switching endpoints, revoked grants, service-version mismatch, schema-too-new, and helper-unavailable states. Remote services must use existing TLS verification and per-device revocation; local fallback is allowed only in the local-helper mode.
6. Sequence implementation behind required APIs. Draft CRUD/send, account/folder lifecycle, cache catalog hydration, mutations/undo/events, and removal of local provider/database construction may require separate tasks. Do not claim remote-only support until the app boots and exercises the accepted core flows with no mailbox DB present.

## Deliverables

- A store/operation matrix with code references and ownership class.
- A startup and first-paint sequence, with bounded network requirements and an explicit cache contract.
- A failure/reconnect state matrix covering local and remote modes.
- Dependency-ordered follow-up tasks with concrete acceptance criteria and safe fallback rules.
- A list of remaining local-only features that can be disabled or deferred without blocking core mail browsing and reading.

## Acceptance criteria

- The design identifies every mailbox DB open/migration/bootstrap assumption in desktop preflight/startup and every service store constructed by `App.Startup`.
- It distinguishes device-authored state from rebuildable service-derived cache and authoritative mailbox data; no user-authored state is proposed for blind deletion or reconstruction.
- It defines remote configuration failure as fail-closed and covers a device with no local service database.
- It specifies a dependency order that reaches a testable remote-only startup slice before broader feature parity work.
- No runtime code changes are included.
