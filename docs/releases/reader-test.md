Postilaatikko 0.3.20 sidebar, composer-undo attachment and durable mailbox-undo test build, built on GitHub from main.
Feature baseline ccd355b (sidebar #52 and mailbox undo #53 merges); the exact source SHA is the main commit that includes these notes, visible in the build workflow run.
Choose arm64 for Apple Silicon (including Mac Studio), amd64 for Intel.
Open the DMG and drag Postilaatikko onto Applications.

- Sidebar: reorder cards and folders with drag handles directly in the sidebar — no modal. Star folders as favorites for the quick rail with distinct icons per entry kind and folder type, and jump with Cmd/Ctrl+1…0. Compact footer.
- Composer: attachments join the draft undo history. Blobs are stored when added, snapshot membership is recorded per state, and restores rehydrate them. A repair migration adds the missing draft attachment blob tables on already-migrated databases.
- Mailbox: undo history is durable for 24 hours across app restarts (global push order, time-bounded retention, no count cap).
- Detached composer/reader window failures are now diagnosable and visible instead of silent.
- CI repairs: registry entries for new composer shortcuts and errcheck compliance on recent code.

Verified with full Go tests including migration and undo regression fixtures, frontend type/lint/unit checks and production build. Native packaging runs in the existing GitHub mac workflow. Native runtime (install, launch, WKWebView behavior) remains unverified until this DMG is checked on a device — please report any startup or window problems before wider use.

Cache schema advances to 59; older binaries reject newer caches. Unmerged branch work (such as saved-search virtual folders, schema 60) is not included. Send Later's existing live delivery/pairing/recovery acceptance gates remain open.
