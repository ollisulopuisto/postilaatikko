# Efficient CI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reduce repeated GitHub runner work while preserving full affected checks and an immediate manual DMG build.

**Architecture:** A conservative Git diff classifier selects frontend/backend jobs without skipping the PR workflow. Each language installs dependencies once; backend compile checks use a disposable embedded asset fixture instead of building the frontend again. Existing status names remain, with Lint aggregating failures. The existing artifact-only macOS workflow gains manual architecture selection and documentation; release workflows remain unchanged.

**Tech Stack:** GitHub Actions, Python stdlib/unittest, npm, Go, Wails.

---

### Task 1: Conservative change selection

Files: create `scripts/ci_scope.py` and `scripts/tests/test_ci_scope.py`.

1. Write table-driven tests for documentation, frontend, Go, shared/unknown files, mixed changes and filenames with newlines.
2. Run `python3 -m unittest discover -s scripts/tests -v`; expect failure before the classifier exists.
3. Implement `classify(paths) -> (frontend, backend)` with unknown changes selecting both; use `git diff --no-renames --name-only -z BASE HEAD` so deletions and both rename sides count.
4. Run tests, including a temporary Git repository and an invalid-base failure.

### Task 2: Share setup and preserve required statuses

Files: modify `.github/workflows/ci.yml`.

1. Keep `Version consistency`, `Lint`, `Type check`, `Test` names. Run version/policy checks always. Select jobs within the workflow, not with top-level path filters.
2. Consolidate frontend lint, check, tests and production build into one install; consolidate Go lint and the full suite into one GTK/Go setup. Keep these jobs parallel.
3. Add PR concurrency cancellation and timeouts. Use an always-run aggregate Lint gate that fails for failed/cancelled/missing selected work.
4. Validate syntax using actionlint and check documentation-only, single-language, mixed, failed and cancelled job outcomes.

### Task 3: Immediate artifact-only hotfix builds

Files: modify `.github/workflows/build-macos.yml`; create `docs/CI.md`; modify `CHANGELOG.md`, `TASKS.md`.

1. Add optional manual architecture (`arm64` default, `amd64`) to the existing workflow, preserving main-push behavior. Cancel stale automatic builds without cancelling manual hotfix builds.
2. Skip automatic documentation-only builds, cache pinned Wails CLI, check versions, verify signature/DMG, label artifacts with source SHA. Preserve existing no-test build behavior; CI is separate.
3. Document exact `gh workflow run build-macos.yml --ref BRANCH -f architecture=arm64` and download commands, validation limitations, default-branch prerequisite and unchanged release checks.
4. Run policy tests, syntax checks, version consistency and whitespace checks; commit implementation, update task review status, push only this task branch.
