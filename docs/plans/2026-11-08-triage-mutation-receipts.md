# HOSTED-SERVER-065 Triage Receipts and Undo Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make helper-side Reply Later, Read Later, Snooze, and Unsnooze writes idempotent, restart-safe, and undoable through the existing durable action history.

**Architecture:** Add a triage mutation handler that uses the existing `ActionReceiptStore`, per-request idempotency digest, operation IDs and generic Undo routes. The helper runtime captures a minimal prior/expected queue snapshot, serializes each account/conversation mutation, applies optional Gmail labels and helper-owned SQL state, and returns a receipt. `mailboxActionOperations.UndoAction` validates the expected current state before restoring the previous membership/deadline and provider label. A committed receipt is replayed exactly; a stranded reservation becomes `outcome_unknown` at helper startup and is not replayed.

**Tech Stack:** Go, `internal/automation/v1`, `internal/automation/client`, `internal/alwayson`, SQLite action receipt ledger.

---

## Implementation steps

1. **Define the triage operation contract.** Add validated operation kinds and account-qualified request types. Require `Idempotency-Key`; hash canonical requests and reuse `ActionReceiptStore.Reserve/Complete`. Return operation ID, changed/state/replay/Undo availability and expiry while keeping existing callers able to decode `changed`.
2. **Add state snapshots and safe helper mutation.** Snapshot current membership or optional Snooze deadline before mutation. Serialize mutations by account/conversation, no-op without creating a new Undo operation when requested state already exists, and include provider-label work inside the helper operation boundary. Persist payload only with a confirmed result; ambiguous work records a terminal unknown receipt without a replayable snapshot.
3. **Implement guarded inverse operations.** Extend the helper action adapter for triage action kinds. Validate snapshot identity and expected state, restore the prior membership/deadline, and reverse Gmail labels when relevant. If current state has changed or expired, fail safely; the existing Undo route will retain an explicit uncertain result.
4. **Mount and compose all routes.** Build the triage handler with the shared receipt ledger/digest in `MailboxReadAPIHandler`; keep existing queue reads/count routes. Ensure all mutation routes share one ledger so Undo latest ordering is global across message and triage operations.
5. **Expose helper client receipts.** Add typed client methods that accept a caller-stable idempotency key and return operation receipts for each mutation. Keep the old convenience methods source-compatible by delegating with a fresh request key.
6. **Prevent duplicate Reply Later Undo and refresh after Undo.** Adapt the existing Wails Reply Later mutation to return the helper operation ID and use helper Undo when connected; only the local-helper-unavailable path should push its existing desktop Undo command. Return typed affected triage state from a confirmed Undo and emit the existing targeted queue event. Read Later and Snooze Wails UX/event updates and durable cursor replay remain HOSTED-SERVER-066/067.
7. **Check the focused build surface.** Run `gofmt`, `git diff --check`, and `GOCACHE=/private/tmp/postilaatikko-go-cache GOPROXY=off go build ./app ./cmd/postilaatikko-helper`. Do not add or run automated tests under the current session instruction.

## Guardrails

- Only use the helper DB and helper provider label adapter for these writes; no desktop mailbox store access is added.
- Keep `mail.write` for mutations/Undo and `mail.read` for history. Do not add a capability.
- Never mark Undo available before provider and helper state agree. Never expose the snapshot through an API response or history summary.
- A new request key that sets an already-current state is a no-op and creates no duplicate Undo entry.
- Do not alter event-cursor schema or desktop queue event emission in this task; HOSTED-SERVER-066/067 own those follow-ups.

## Completion evidence

- The helper handler reserves before provider effects, returns the same stored result for an identical key/request, rejects key reuse with another request, and does not re-run interrupted work.
- Queue mutation Undo payloads use the shared action ledger and are consumed by `/v1/undo/latest` and `/v1/undo/operations/{id}`.
- Local-helper and remote clients use one helper implementation; existing non-mutation queue reads/counts remain unchanged.
- Focused helper/app builds and whitespace checks pass; tests are not run unless explicitly requested.
