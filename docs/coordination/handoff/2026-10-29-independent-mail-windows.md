# P-013: independent mail windows

Review candidate on `codex/P-013-independent-mail-windows`, worktree
`/private/tmp/postilaatikko-p013`. Implementation is `a55e797` plus `b922775`,
with `origin/main` through `1817a78` merged into the task branch. This is not
an integration or release claim.

## Result

- The main mail view has **New mail window**. A child opens at the current
  real, unified or virtual folder and can select another folder independently.
  Children also offer New mail window, New message, list-width and thread-focus
  controls. In a child, Cmd/Ctrl+Shift+N opens another mail window; `/`, list
  navigation, Enter, compose and reader reply/scroll keys are supported.
- Each child process owns its list/search, checked rows, per-folder selection
  and scroll memory, reader mode, width and keyboard focus. Presentation state
  is session-local; reopening a closed child does not restore its session.
  The initial reader preference is copied from the primary window.
- The parent exclusively owns database connections, sync, provider operations,
  body/attachment fetching, mutations and durable drafts. Children start neither
  another sync engine nor another database. Cross-account operations retain the
  existing confirmed-copy safety path.
- Children authenticate through the existing IPC token passed over stdin.
  Folder startup checks validate ownership. A typed operation allowlist exposes
  required mail functions, including saved-search creation/cache reads, while
  rejecting primary UI-state writes and account/application administration.
  Local adapters handle UI-state and sort changes without persisting them to
  the primary window.
- Bounded parent mail events refresh children, including saved-search cache
  updates. Focus refresh preserves an active search. Existing parent shutdown
  broadcasts close children; closing a child releases its connection only.

## Verification

Executed on macOS after merging the current main:

- `go test ./...` and `go build ./...`: pass.
- `go test -race ./app ./internal/ipc ./internal/readerbridge`: pass.
- `go vet ./app ./internal/ipc ./internal/readerbridge`: pass.
- `npm test`: all 180 tests pass, including independent backend UI-state/sort
  adapters and immutable account-qualified navigation/selection.
- `npm run check`: 0 errors; two pre-existing title-bar accessibility warnings;
  eight shortcut-registry tests pass. The sandbox reports a blocked Vite HMR
  listener during this check, but it does not fail the tests.
- Targeted ESLint on the new frontend entry, component, state helper and tests:
  pass. `npm run build` and `git diff --check`: pass. Existing Rollup chunk-size
  and inbox-comment warnings remain.
- Temporary Chromium harness against the production assets and synthetic IPC
  responses: two pages bootstrap and retain independent navigation, selection,
  widths and search; background folder-sync preserves the query; New window
  and compose route the expected account/scope; saved-search dialog and cached
  view work; a narrow viewport shows the selected reader; no primary UI-state
  writes or browser errors. The harness first caught a null-thread error when
  switching folders with a reader open. The scoped selected-thread binding
  fixes that path, and the same harness passes afterward.
- The Go allowlist/cache-event regressions failed before the saved-search
  integration fix, then passed. Tests also cover rejected shared-state writes,
  exact RPC arguments, account/folder isolation and bounded events.

Tests used a writable temporary Go cache. IPC/mock-server tests and Chromium
needed local socket permission outside the filesystem sandbox. No live mailbox
data was used for the browser smoke test.

## Remaining checks and limits

1. In a packaged Wails build, open multiple mail windows from real, unified and
   virtual folders. Navigate/search/select independently, change widths and
   reader modes, and verify that the primary window retains its layout.
2. Close a child while the primary keeps running, then quit the primary with
   children open. Verify native activation, window close controls and shutdown.
3. Exercise real Gmail/IMAP search, cross-window read/star/move/trash/undo,
   saved-search creation, on-demand body loads, reply/forward, encrypted mail
   and native attachment dialogs. Browser mocks do not verify those providers
   or native dialogs. The full primary command-panel/settings shell is outside
   this mail-window view.
4. Existing IPC replies remain limited to 9 MiB; larger replies return an
   explicit error. Mail-event payloads above 256 KiB are skipped and logged;
   refocusing refreshes the cached list. Paged transport remains future work.
5. Wails v2.15 binding generation was inspected; only relevant declarations and
   ReaderConfig fields were retained. A full regeneration exposes an existing
   `drafthistory.Blob.content` discrepancy (`number[]` generated vs the composer's
   base64 string contract), producing two composerApi type errors. Unrelated
   generator changes were discarded; this task preserves the current main's
   composer binding behavior. Resolve that discrepancy separately before
   treating a fresh full regeneration as a clean type-check baseline.

The coordinator owns review, integration and the requested GitHub DMG build.
