# Virtual Folders Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete virtual folders as saved sets of mail folders that display unread conversations in the sidebar and support create, edit, reorder, and delete.

**Architecture:** Persist virtual folder metadata and folder membership in SQLite, expose the operations through Wails bindings, then add reactive frontend state and sidebar/editor flows. Reuse the existing message-list rendering and multi-folder conversation queries, and profile the data flow for avoidable calls and retained data before optimizing it.

**Tech Stack:** Go, SQLite (`database/sql`), Wails-generated TypeScript bindings, Svelte 5, TypeScript.

---

### Task 1: Complete backend storage and API

**Files:**
- Modify: `internal/virtualfolder/store.go`
- Modify: `app/virtual_folder.go`
- Modify: `internal/database/migrations.go` if adopting the numbered migration convention

Add proper virtual-folder membership foreign keys, transactional create/update/delete behavior, validation, and stable empty arrays. Ensure update of a missing ID returns a not-found error. Preserve unread filtering and use the existing folder-ID conversation queries.

### Task 2: Refresh Wails bindings

**Files:**
- Generate: `frontend/wailsjs/go/app/App.js`
- Generate: `frontend/wailsjs/go/app/App.d.ts`
- Generate: `frontend/wailsjs/go/models.ts`

Regenerate bindings from the application methods and confirm the model and operations are represented.

### Task 3: Add frontend virtual-folder state and actions

**Files:**
- Create: `frontend/src/lib/stores/virtualFolders.svelte.ts`
- Modify: `frontend/src/lib/components/sidebar/Sidebar.svelte`
- Modify: `frontend/src/App.svelte`
- Create: `frontend/src/lib/components/sidebar/VirtualFolderDialog.svelte`

Load virtual folders, show unread counts, select a virtual folder as a message-list source, and wire create/edit/delete/reorder UI to Wails.

### Task 4: Integrate virtual folder conversation loading

**Files:**
- Modify: `frontend/src/App.svelte`
- Modify: `frontend/src/lib/components/list/MessageList.svelte`
- Modify: `frontend/src/lib/components/list/ConversationRow.svelte` only if account/folder identity needs adaptation

Route virtual-folder selection through the existing list/viewer and mutation paths. Ensure unread actions refresh membership results and counts correctly.

### Task 5: Optimize responsiveness and memory use

**Files:**
- Modify: `frontend/src/lib/stores/virtualFolders.svelte.ts`
- Modify: `internal/virtualfolder/store.go`
- Modify: relevant sidebar/list components

Avoid polling each virtual folder individually when a batch count can serve the sidebar; refresh counts after sync and read-state mutations, debounce/coalesce refreshes, avoid retaining duplicate conversation arrays, and page results through the existing list API.

### Task 6: Update product documentation

**Files:**
- Modify: `ROADMAP.md`
- Modify: `CHANGELOG.md`

Mark virtual folders complete only after the backend, binding, sidebar and message list paths are connected. Keep scheduled send listed as not started unless implemented separately.
