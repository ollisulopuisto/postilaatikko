# Split macOS and all-platform release workflows Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Publish each macOS DMG as soon as its native build finishes while Linux and Windows release builds run independently.

**Architecture:** Keep the existing version-tag release workflow for Linux and Windows and move native macOS packaging into a dedicated tag-triggered workflow. Both workflows upload disjoint assets to the same GitHub Release; release creation must tolerate either workflow winning the race. The macOS arm64 and amd64 jobs publish independently so a slow Intel runner cannot hold up the Apple Silicon DMG.

**Tech Stack:** GitHub Actions YAML, GitHub CLI, existing Wails/Go/npm release build and DMG packaging.

---

### Task 1: Claim and document workflow ownership

**Files:** `TASKS.md`; this plan.

Record the isolated branch/worktree and acceptance conditions before workflow edits. Commit this checkpoint separately.

### Task 2: Separate release build matrices

**Files:** `.github/workflows/release.yml`; create `.github/workflows/release-macos.yml`.

Keep Linux/Windows builds and publication in `release.yml`. Add a tag-triggered macOS workflow with native arm64 and Intel runners; build a DMG containing `Postilaatikko.app` and an `/Applications` symlink, plus the existing ZIP asset. Each architecture uploads its own assets after its build, with unique asset names and matching release tag.

### Task 3: Make release creation race-safe

**Files:** both release workflow files; optionally a shared script only if the workflow commands become nontrivial.

Either workflow may create the GitHub Release first. If creation loses a race, confirm the release now exists before uploading. Keep tag validation and generated notes, and do not let the macOS workflow publish a different tag or source revision.

### Task 4: Update operator instructions and verify configuration

**Files:** `docs/CI.md`; `TASKS.md`.

Document that pushing a `v*` tag starts both release workflows and macOS assets appear as each architecture completes; preserve the artifact-only hotfix path. Run actionlint, inspect parsed trigger/matrix/permissions, and check whitespace. Commit reviewable workflow and documentation checkpoints.

**Acceptance:** A tag creates one public release; Linux/Windows assets no longer wait for macOS jobs; arm64 and amd64 DMGs upload independently; each DMG has an `/Applications` shortcut; release creation races do not fail either publisher; hotfix artifact workflow remains release-isolated.
