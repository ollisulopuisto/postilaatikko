# Cross-platform helper packaging plan

**Goal:** Ship `postilaatikko-helper` beside the desktop binary on supported package types so a later bootstrap handshake can resolve one fixed sibling executable path.

**Scope:** Build/package wiring only. Keep schema migrations, service startup policy, and desktop behavior unchanged.

## Task 1: Build the helper for app targets

- Build the helper for macOS as today, and for Linux and Windows with the same target architecture as the Wails application.
- Build the helper before `wails dev` so development runs have a deterministic helper artifact.
- Keep the helper output name `postilaatikko-helper` on Unix and `postilaatikko-helper.exe` on Windows.

## Task 2: Include the helper in installers and app packages

- Install the helper in the same directory as the desktop executable in the Windows NSIS installer.
- Include the helper in Linux release archives and install it to the same binary directory as `eterno-mail`.
- Include the helper in Flatpak app images and both local and Flathub-oriented manifests.
- Preserve macOS bundle placement at `Contents/MacOS/postilaatikko-helper`.

## Task 3: Verify packaging contracts

- Cross-build the helper for Linux and Windows where the current Go toolchain permits.
- Inspect installer/archive/Flatpak commands to confirm the installed paths are siblings.
- Run shell syntax checks for modified scripts and `git diff --check`; do not run automated tests per session instructions.

## Acceptance

- Each production package contains the helper at a documented fixed path beside the app executable.
- The development target builds the helper deterministically before starting Wails.
- Build/install failures are explicit if the helper cannot be produced; no package silently claims to contain it when absent.
- No database, schema, or migration ownership behavior changes in this task.
