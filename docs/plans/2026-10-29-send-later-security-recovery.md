# Send Later Security and Recovery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Verify the restored P-011 safety boundaries with deterministic regression tests and fix reproduced failures.

**Architecture:** Exercise real encrypted commands and private SQLite queues through two connections. Use channel-controlled delivery and HTTP responses to reproduce in-flight cancellation, stale status and replay behavior without a real mail provider.

**Tech Stack:** Go testing, SQLite, NaCl boxes, httptest.

---

### Task 1: Recovery and delivery authorization

Create `internal/scheduledsend/security_recovery_test.go`; reuse baseline identity/command helpers. Table-test transport classifications, retry limit, interruption recovery and stale worker completion. Confirm ambiguous and authorization-required jobs never automatically resend, and manual resolution is revision-bound.

Run `GOCACHE=/private/tmp/postilaatikko-p011-gocache go test ./internal/scheduledsend` after adding coverage. Investigate a failing safety assertion before any production edit; record red/green evidence for any reproduced defect.

### Task 2: Client and transport races

Create `internal/scheduledsend/security_client_test.go`. Use channel gates and independent SQLite connections for claims, cancellation while sending, stale receipt/status, and Send Now arbitration. Verify an offline request retains its exact encrypted bytes and an old encrypted acknowledgement cannot confirm a fresh status query.

Run the focused tests, then `go test -race ./internal/scheduledsend` and `go vet ./internal/scheduledsend ./cmd/postilaatikko-helper` with the isolated cache. Production code changes require regression-first verification and an Unreleased changelog entry.

### Task 3: Review handoff

Run the project-required Go suite, whitespace checks and affected helper build. Update `docs/SEND_LATER.md` with automated coverage and explicit remaining native/provider gates; update P-011-SEC in `TASKS.md`, commit and push the review checkpoint. Do not merge or claim real delivery, keychain/login/reboot checks.
