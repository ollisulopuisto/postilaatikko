# UX-026 Reply Later handoff

Owner: Codex Sol 6.1 Medium. Branch: `codex/UX-026-reply-later-ui`.
Worktree: `/private/tmp/postilaatikko-ux026`. Base: origin/main `c766bc3`.

## Ownership

Implement Reply Later add/remove controls, sidebar view/count and virtual-folder
queue filtering. Do not edit `App.svelte`, `main.go`, `app/undo.go` or
`internal/undo/`; UX-044 owns those files. Record required App.svelte hooks here.
Use existing UX-025 durable membership and normal conversation list/viewer.

## Status

Implementation committed at `c576441`; ready for coordinator integration. The original agent stopped at
its usage limit; Sol 6.1 Medium resumed the existing claimed worktree without
resetting its changes.

## Implemented

- Conversation-row clock control and single-conversation context-menu action add
  or remove explicit Reply Later membership, with operation-targeted Undo.
- Reply Later appears in the Mail card, can be moved/hidden/pinned like other
  entries, and displays the durable queue count. It opens through the existing
  generic unified-folder callback; no App.svelte change is required.
- Unified Reply Later list/count/search apply queue membership across enabled
  accounts. The view does not trigger a full sync across every folder.
- Virtual-folder quick filters now pass unread/Reply Later to online search.
  Queue membership is intersected before Gmail metadata hydration, and provider
  ID scans are bounded to ten pages per request. Filtered virtual views bypass
  the unfiltered cached first paint and full-view online counter.
- Queue operations emit `replyLater:changed` and `undo:history-changed`.
- Mail-window scope/RPC allowlists support Reply Later; membership changes are
  forwarded to independent mail windows.

## Integration hooks and remaining limits

- UX-044 owns App.svelte and undo history. Merge this branch with its work;
  `undo:history-changed` is already emitted after queue operations are pushed.
- Keep the existing generic `onUnifiedSpecialFolderSelect` callback unchanged.
- Queue refs whose local conversation no longer exists stay durable and counted;
  the current mail-list view can display only locally indexed conversations.
- The external agent API adapter remains dependent on the authorized transport
  work; this task does not introduce an unauthenticated endpoint.

## Verification

- `GOCACHE=/private/tmp/go-cache-postilaatikko-ux026 go build ./app`: passed.
- `npx svelte-check --tsconfig ./tsconfig.json`: zero errors, two pre-existing
  titlebar double-click accessibility warnings.
- `npm run build`: passed; existing titlebar/PURE comment/chunk-size warnings.
- `git diff --check`: passed.
- Tests were not run under coordinator instruction. Live macOS/Gmail behavior
  remains an installed-build acceptance check.
- Origin/main advanced to `b8b954b` (PR #66) during implementation. This branch
  remains on `c766bc3`; coordinator will resolve overlapping count/event/docs
  changes during integration.

## Coordinator integration checkpoint (2026-10-30)

- Cherry-picked UX-026 onto current `origin/main` `b8b954b` in the combined
  candidate branch `codex/integration-reply-undo`, commit `6081465`.
- Combined with UX-044 Undo History (`f5ec230`), including the newer main's
  runtime event invalidation for shared online counts. `go build ./...`,
  Svelte check (0 errors; two existing titlebar warnings), frontend production
  build, version validation and `git diff --check` pass.
- UX-026's API adapter and stale local conversation rendering limitations above
  remain. Live Gmail and native-menu behavior still need installed-build review.

### CI integration follow-up

- PR #68 exposed a stale `targetedUndoConsumers` assertion that expected the removed `MessageList.handleBulkArchive` callback. Updated the regression check to assert the current `runListAction`/`onBulkAction` delegation contract instead.
- Targeted verification: `node --test frontend/tests/targetedUndoConsumers.test.mjs` (9 passed); `git diff --check` passed.
- A second CI rerun exposed another stale regression-test harness: `clearSearch` now notifies the optional `onSearchQueryChange` callback. Added the no-op callback to that isolated harness; no production behavior changed.
- Targeted verification now covers both affected files: `node --test frontend/tests/searchMutationFlow.test.mjs frontend/tests/targetedUndoConsumers.test.mjs` (15 passed).
