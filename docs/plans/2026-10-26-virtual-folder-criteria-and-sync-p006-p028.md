# Virtual Folder Criteria & Default Sync Handoff (P-006 & P-028)

**Authors:** Antigravity  
**PR:** [#13](https://github.com/ollisulopuisto/postilaatikko/pull/13)  
**Merged Commit:** `8d0eed4`  
**Date:** 2026-10-26  

---

## 1. Overview & Problem Statement

This work resolves two high-priority user issues:

1. **Virtual Folder Criteria & Empty State Bug (Task P-006)**:
   - *Previous behavior*: In Postilaatikko v0.3.10–v0.3.14, virtual folders only saved a static array of member folder IDs. At the query layer (`app/virtual_folder.go`), message loading called `ListConversationsByFolderIDs(..., "unread")` with a hardcoded unread filter. If an email within an included folder was read, it was hidden. If all messages were read, the virtual folder appeared completely empty with no explanation.
   - *New behavior*: Virtual folders now persist explicit criteria facets across accounts:
     - `folder_mode`: `'include'` (only selected folders) or `'exclude'` (all folders except selected).
     - `read_status`: `'all'`, `'unread'`, or `'read'`.
     - `star_status`: `'all'`, `'starred'`, or `'unstarred'`.
     - `reply_status`: `'all'`, `'replied'`, or `'unreplied'`.
     - `date_window`: `'all'`, `'7d'`, `'30d'`, `'90d'`, or `'1y'`.
   - Criteria apply consistently across conversation listing, search, and sidebar badge counts. The criteria can be updated at any time in the GUI.

2. **Default Sync Period All Messages (Task P-028)**:
   - Routine sync in Postilaatikko is lightweight and headers-only (v0.3.12+).
   - Accounts previously defaulted to 180 days (or 30 days in fallback code), which truncated local history unexpectedly.
   - The default `SyncPeriodDays` across models, OAuth flows, and dialogs is now `0` (All Messages).

---

## 2. Database Schema & Migration 52

Migration 52 (`internal/database/migrations.go`):
```sql
-- Migration 52: Add criteria columns to virtual_folders and update legacy sync period defaults to 0
ALTER TABLE virtual_folders ADD COLUMN folder_mode TEXT NOT NULL DEFAULT 'include';
ALTER TABLE virtual_folders ADD COLUMN read_status TEXT NOT NULL DEFAULT 'all';
ALTER TABLE virtual_folders ADD COLUMN reply_status TEXT NOT NULL DEFAULT 'all';
ALTER TABLE virtual_folders ADD COLUMN star_status TEXT NOT NULL DEFAULT 'all';
ALTER TABLE virtual_folders ADD COLUMN date_window TEXT NOT NULL DEFAULT 'all';

UPDATE accounts SET sync_period_days = 0 WHERE sync_period_days IN (30, 180);
```

### Models (`internal/virtualfolder/model.go`)
- `FolderMode`: `FolderModeInclude` (`"include"`), `FolderModeExclude` (`"exclude"`).
- `ReadStatus`: `ReadStatusAll` (`"all"`), `ReadStatusUnread` (`"unread"`), `ReadStatusRead` (`"read"`).
- `ReplyStatus`: `ReplyStatusAll` (`"all"`), `ReplyStatusReplied` (`"replied"`), `ReplyStatusUnreplied` (`"unreplied"`).
- `StarStatus`: `StarStatusAll` (`"all"`), `StarStatusStarred` (`"starred"`), `StarStatusUnstarred` (`"unstarred"`).
- `DateWindow`: `DateWindowAll` (`"all"`), `DateWindow7D` (`"7d"`), `DateWindow30D` (`"30d"`), `DateWindow90D` (`"90d"`), `DateWindow1Y` (`"1y"`).
- `normalizeCriteria()` enforces valid fallback values when empty.

---

## 3. Query Engine Architecture (`internal/message/virtual_folder_queries.go`)

Virtual folder message querying avoids the old hardcoded unread limitation by evaluating SQL criteria on conversations:

```go
type VirtualFolderCriteria struct {
    FolderIDs   []string
    ReadStatus  string // all, unread, read
    ReplyStatus string // all, replied, unreplied
    StarStatus  string // all, starred, unstarred
    DateWindow  string // all, 7d, 30d, 90d, 1y
}
```

### Dynamic SQL Filters
- **Member Folder Resolution**: Handled via `store.QueryFolderIDs(vf)`:
  - If `folder_mode == 'exclude'`, member folders are computed as all active folders minus the excluded IDs.
  - If `folder_mode == 'include'`, the saved folder IDs are used directly.
- **Date Window**:
  - Filter applied on the conversation's latest message activity timestamp (`MAX(m.date) >= datetime('now', '-7 days')`, etc.).
- **Read Status**:
  - `unread`: `SUM(CASE WHEN m.is_read = 0 THEN 1 ELSE 0 END) > 0`
  - `read`: `SUM(CASE WHEN m.is_read = 0 THEN 1 ELSE 0 END) = 0`
- **Star Status**:
  - `starred`: `SUM(CASE WHEN m.is_starred = 1 THEN 1 ELSE 0 END) > 0`
  - `unstarred`: `SUM(CASE WHEN m.is_starred = 1 THEN 1 ELSE 0 END) = 0`
- **Reply Status**:
  - Checks if any message in the thread has the replied/answered flag or header reference.
- **Search & Unread Counts**:
  - `SearchVirtualFolder` and `GetVirtualFolderSearchCount` share the exact same criteria generator so in-folder search results never diverge from folder list results.
  - `virtualFolderStore.UnreadCounts()` counts unread matching the saved criteria rather than assuming an unread filter is active.

---

## 4. Frontend & GUI Changes

1. **`VirtualFolderDialog.svelte`**:
   - Added interactive form controls for:
     - Folder Mode (Include selected vs Exclude selected)
     - Read Filter (All, Unread only, Read only)
     - Star Filter (All, Starred only, Unstarred only)
     - Reply Filter (All, Replied only, Not replied)
     - Date Range (All time, 7 days, 30 days, 90 days, 1 year)
   - When editing an existing virtual folder, the `$effect` pre-populates the existing criteria for easy modification.
   - Full localization in `frontend/src/lib/i18n/locales/en.json` and `fi.json`.

2. **`MessageList.svelte`**:
   - Added reactive criteria change detection: switching virtual folder criteria triggers immediate list reload.
   - Added spinning animation to the toolbar sync button during manual sync (`manualSyncing || syncing`).

3. **`AccountForm.svelte` & `AccountDialog.svelte`**:
   - Default `syncPeriodDays` initialized to `'0'` (All Messages).

---

## 5. Notes for Other Agents & Claude

- **Replay Migrations in Unit Tests**: If you add columns in future migrations (e.g. Migration 53+), ensure you update the teardown blocks in `internal/database/database_test.go` (`TestMigrationV32`, `TestMigrationV33`) to drop newly added columns before `Migrate()` is replayed; otherwise, replay tests will fail with `duplicate column name`.
- **Wails Model Synchronization**: If changing `VirtualFolder` struct fields in Go, remember to update `frontend/wailsjs/go/models.ts` and verify with `npm --prefix frontend run check:shortcuts` and `npm --prefix frontend run check`.
