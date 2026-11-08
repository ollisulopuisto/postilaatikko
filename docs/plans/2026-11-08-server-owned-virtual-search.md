# Server-owned virtual-folder results implementation plan

**Goal:** Make the always-on helper own virtual-folder result-cache refresh and reads so a thin desktop can paint saved views quickly without opening the mailbox database.

**Architecture:** The helper already owns saved-view definitions, a bounded (200 conversation) membership/order cache, and the shared `mailservice.Service`. Add an authenticated cache-read endpoint that hydrates cached refs into current conversation metadata; periodically refresh every saved-view cache in the helper lifecycle; route desktop cache reads and virtual-folder searches through the helper when connected. Preserve the existing Wails cache-first render, with the normal bounded search page refreshing the visible list after the cache paint.

**Tech Stack:** Go, `internal/alwayson`, `internal/automation/v1`, `internal/automation/client`, `internal/mailservice`, Svelte/Wails bindings, SQLite.

---

### Task 1: Add red tests for cached-view API behavior — complete

**Files:**
- Test: `internal/alwayson/catalog_api_test.go`
- Test: `internal/automation/v1/virtual_folder_cache_test.go`

**Steps:**
1. Build a migrated helper DB with an account, folder, saved view, message, and a known cache order/timestamp.
2. Assert the helper operation hydrates cached refs in cache order with current unread/snippet metadata and returns freshness/has-cache metadata.
3. Exercise authenticated read routing and verify an unknown view returns not-found without leaking mailbox data.
4. Run the focused tests and confirm the missing API operation/route makes them fail.

### Task 2: Implement the helper-owned cached result endpoint — complete

**Files:**
- Modify: `internal/automation/v1/virtual_folders.go`
- Modify: `internal/automation/v1/read.go` or a focused new cache-route file
- Modify: `internal/alwayson/catalog_api.go`
- Modify: `internal/automation/client/client.go`

**Steps:**
1. Add a response containing view ID, hydrated conversation summaries, cache timestamp, and whether a cache existed.
2. Add a `mail.read`-authorized `GET /v1/views/{id}/cache` route and client method.
3. Have `SyncRuntime` load the service-owned cache, compute it once if absent, and hydrate current conversation rows in stored order.
4. Verify API tests, including empty and stale cache cases.

### Task 3: Refresh cached views from the independent helper lifecycle — complete

**Files:**
- Modify: `internal/alwayson/sync.go`
- Test: `internal/alwayson/sync_test.go` or a focused cache-maintenance test

**Steps:**
1. Add cancellable helper maintenance that refreshes all saved-view caches immediately at startup and every five minutes thereafter.
2. Process views sequentially, log per-view failures, and continue refreshing other views; never fetch message bodies or attachments.
3. Stop the refresher during helper shutdown and test initial refresh, periodic refresh, and cancellation with an injected interval/clock seam.

### Task 4: Route desktop cache reads and virtual-folder search through the helper — complete

**Files:**
- Modify: `app/mailbox_service.go`
- Modify: `app/virtual_folder.go`
- Modify: `app/mail_search.go`
- Test: `app/mail_search_service_test.go`

**Steps:**
1. Add the desktop connector call for cached view reads and use it from `GetVirtualFolderCachedView` when the helper is connected.
2. Keep the existing local path only for helper-unavailable operation; surface authorization/API failures.
3. Route virtual-folder searches through `ScopeVirtualFolder` for empty typed queries and `ScopeSearch` within the view for typed queries, preserving existing result conversion, filter, cursor, and cancellation behavior.
4. Verify the Wails frontend still paints cache results before the fresh bounded search completes; avoid expanding cache beyond its existing 200-conversation limit.

### Task 5: Update ownership documentation and verify — complete

**Files:**
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`
- Modify: `CHANGELOG.md`
- Modify: `TASKS.md`

**Steps:**
1. Record that cache storage, refresh, and connected desktop reads are service-owned; identify remaining local fallback, unread-count, and live-device gaps precisely.
2. Run focused Go tests for helper API/cache, app virtual-folder search, and existing mail search; build desktop/helper and run `git diff --check`.
3. Commit implementation, record its commit and verification, commit the ledger, then fast-forward and push `main`.
