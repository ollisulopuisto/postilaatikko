# Command and Shortcut Registry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make one typed command catalog the source for core command identity, shortcut presentation, command-palette entries, and handler registration.

**Architecture:** Keep contextual execution in `App.svelte` and list-target resolution in the list action layer, since both depend on live component state. Put stable IDs, labels, icons, palette categories, shortcut references, availability rules, and dispatch registration contracts in one typed catalog. Generate the command palette from that catalog and make keyboard/action surfaces dispatch through the same IDs; extension-defined shortcuts remain extension-owned and are composed at runtime.

**Tech Stack:** TypeScript, Svelte 5, existing `$lib/keyboard/registry`, action target/executor modules, command-palette store.

---

### Task 1: Define the command catalog

**Files:**
- Create: `frontend/src/lib/commands/catalog.ts`
- Modify: `frontend/src/lib/keyboard/registry.ts`
- Modify: `frontend/src/lib/actions/listActions.ts`

1. Define typed core command descriptors with stable ID, label key, icon, category, optional shortcut ID, optional palette visibility, and optional availability class.
2. Register callable core app, view, theme, reply, and mail-action commands once, preserving existing IDs and key combinations.
3. Keep provider/UI behavior in injected handlers; command metadata must not import `App.svelte` or component state.
4. Link list action execution definitions to catalog IDs, removing copied labels/icons/shortcut identity where the catalog already supplies them.
5. Keep extension shortcut discovery and its runtime composition unchanged.

### Task 2: Centralize handler registration and dispatch

**Files:**
- Modify: `frontend/src/lib/stores/commandPalette.svelte.ts`
- Modify: `frontend/src/App.svelte`

1. Replace unrelated handler and availability maps with a typed registration API keyed by catalog command IDs.
2. Reject or report unknown core IDs at registration time while permitting explicitly namespaced extension commands.
3. Register each core command with one handler and optional availability callback; return unregister functions for lifecycle cleanup.
4. Keep target resolution and error/toast handling in the app executor, and keep commands without a visible shortcut dispatchable by their stable ID.
5. Route the existing keyboard paths for callable commands through the shared dispatcher where they have no extra event-specific parameters; retain direct contextual key handling where key state changes the operation.

### Task 3: Generate the command palette from the catalog

**Files:**
- Modify: `frontend/src/lib/components/command/CommandPalette.svelte`
- Modify: `frontend/src/lib/keyboard/format.ts` only if a catalog lookup helper is needed.

1. Replace the hand-built core palette list with catalog descriptors, localized labels/categories, registry icons, and shortcut formatting.
2. Preserve dynamic active-view and theme labels, plus existing visibility/availability behavior.
3. Keep filtering, selection, keyboard navigation, and extension view commands working.
4. Ensure palette entries use the same registry IDs as shortcut hints and list action buttons.

### Task 4: Align action surfaces and verify

**Files:**
- Modify: `frontend/src/lib/components/list/MessageList.svelte` as needed.
- Inspect: `frontend/src/lib/components/common/MessageContextMenu.svelte`, `frontend/src/lib/components/shortcuts/ShortcutsDialog.svelte`, `frontend/src/lib/components/shortcuts/ShortcutHint.svelte`.

1. Replace remaining duplicated command labels/icons/shortcut IDs on action surfaces with catalog lookups while preserving state-dependent labels.
2. Confirm command palette, menu hints, list selection actions, and keyboard actions resolve the same IDs.
3. Run `npm run check`, targeted ESLint, and `npm run build` when frontend dependencies are available; run `git diff --check`.
4. Do not run automated tests in this session. Record unavailable checks and remaining runtime/device coverage in `TASKS.md`.
5. Commit the feature and update the task ledger for coordinator integration.
