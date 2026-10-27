# Gmail navigation and mailbox rebuild implementation plan

**Goal:** Ship customizable sidebar cards, Gmail-first operations, cross-account server search and API-backed virtual folders with automatic header refresh in a GitHub-built test DMG.

**Architecture:** Persist typed navigation cards independently of mailbox data. Use Gmail provider identifiers explicitly, never infer IMAP UIDs from Gmail IDs. Keep Gmail queries and message metadata on the server; reuse the existing local conversation renderer and explicit body fetching. Non-Gmail accounts retain IMAP. Mutations must not retry through a different transport after ambiguous acceptance; cross-account moves confirm destination copies before deleting sources.

**Tech stack:** Go, SQLite, Gmail REST API, Svelte 5, Wails, GitHub macOS Actions.

1. Navigation (`frontend/src/lib/sidebar/cards.ts`, new card store/component, Sidebar/ExtensionRail, `internal/appstate/model.go`): write normalization/reorder/move/shortcut tests; run red; implement persisted cards, drag and keyboard controls, collapse/hide/restore, rail favorites and Cmd/Ctrl+1…0. Run Node tests and frontend checks; commit.
2. Gmail transport (`internal/gmail`, provider store, `internal/sync`, app actions/compose): inventory each existing read/mutation path. Add HTTP regression tests before wiring metadata, raw bodies, labels, mutations, import/send/drafts. Preserve provider identities, account isolation, MIME, explicit-fetch policy and error/reauthorization behavior. Run focused Go tests/race checks; commit.
3. Global and virtual search (`app/search.go`, `app/virtual_folder.go`, new query compiler and search UI): reproduce empty local-cache results, add account-scoped Gmail queries including include/exclude folder union and status/date semantics. Implement bounded pagination, partial errors and explicit loading/fetching states. Verify remote-only messages can open and be acted upon. Commit.
4. Refresh (`app/sync.go`, scheduler, message list): test request coalescing, stale/open refresh, all-folder background metadata and exclude-mode membership. Show cached results immediately; bodies/attachments remain on demand. Commit.
5. Integration: regenerate bindings; frontend check/lint/build, affected Go packages and browser smoke; document live-provider limitations. Update TASKS with commits. Push task branch and enable GitHub mac-test-build trigger; verify both DMGs published with app + Applications link. No push to main.

Use test-first, focused commits and TASKS updates throughout. This request supersedes earlier ordering: sidebar, Gmail operations, global search, background refresh, then DMG; virtual-folder API correctness is a release gate.
