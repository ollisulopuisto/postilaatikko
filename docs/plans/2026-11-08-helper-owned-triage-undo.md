# Helper-Owned Triage Mutation Undo Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the always-on helper the durable owner of Undo for Reply Later, Read Later, Snooze, and Unsnooze mutations in both local-helper and remote-desktop modes.

**Architecture:** Extend the existing helper action-receipt and action-undo contract rather than building a second Undo stack. A triage mutation stores a privacy-minimal snapshot of prior and expected queue state, performs provider-label changes where applicable, and returns the helper operation ID only after a confirmed state transition. Existing authenticated `/v1/undo/*` endpoints claim and reverse these operations with compare-before-restore guards. Desktop Wails bindings adapt the helper result into the existing operation-scoped toast and durable Undo history UI; the desktop-local `undoStack` is used only when the local helper is unavailable.

**Tech Stack:** Go, `internal/automation/v1`, `internal/automation/client`, `internal/alwayson`, SQLite action receipt ledger, `app/mailbox_service.go`, Wails events.

---

## Current-state evidence

- Reply Later's helper route returns only `{changed}` (`internal/automation/v1/reply_later_http.go`). The client exposes only a bool. `App.ChangeReplyLaterWithUndo` calls the helper, then pushes a desktop-memory command to `undoStack`; it also requires `replyLaterStore != nil`, which prevents the remote-only path from being usable without a local mailbox database.
- Read Later and Snooze have helper-owned stores/routes, but their responses expose no operation ID and their Wails operations do not create durable Undo entries (`internal/automation/v1/triage_http.go`, `app/triage.go`).
- `SyncRuntime.SetReplyLaterMembership` and `SetReadLater` apply optional Gmail labels before writing helper queue state. These provider calls must be included in the confirmed/uncertain operation boundary; Undo must reverse the label and queue membership together and must not claim success if provider outcome is ambiguous.
- Snooze state is a helper-owned deadline. Replacing a deadline and removing a snooze both need the previous deadline captured. The background expiry worker can remove due rows, so Undo must compare current state with the operation's expected state before restoring anything.
- `mail_action_operations` already persists action kind, request hash, operation ID, response, privacy-minimal undo payload and an expiry. `/v1/undo/history`, `/latest`, and `/operations/{id}` already implement bounded history, atomic claims, idempotent Undo, and uncertain outcomes. The event cursor persists content-free affected message refs but does not yet identify queue refs.
- `app/undo.go` already routes global and operation-scoped Undo through the helper when connected; `UndoHistoryPanel` consumes the helper action summary. The remaining integration is to surface queue operation receipts and reconcile queue state/events.

## Required invariants

1. Queue mutations are explicit state-setting requests. A repeated request that finds the requested state already present is a no-op and must not create another Undo operation.
2. Every changed operation is reserved in the helper receipt ledger before provider effects. Same key + same canonical request replays the same operation result; same key + different request conflicts. An interrupted provider/state transition is recorded as uncertain and is never replayed automatically.
3. Undo payloads contain only account ID, conversation ID, action kind, prior queue state, and expected post-action queue state/deadline. No message bodies, headers, recipients, credentials, bearer tokens, or raw idempotency keys are stored or returned.
4. Undo compares current state against the expected post-action state before changing it. If another operation changed the queue entry, or a snooze expired, Undo refuses rather than overwrite newer state.
5. Reply Later and Read Later provider labels are part of their operation. Mutation/Undo is available only after provider and helper state agree; uncertain provider outcomes stay visible as unknown and are not silently retried.
6. A helper-accepted mutation never adds a desktop-local Undo command. Helper authorization, capability, and network errors fail closed; only true local-helper unavailability may use existing local behavior.
7. Immediate mutation and Undo update open queue views through typed queue events. The durable action event cursor can replay affected account/conversation refs after reconnect, without a full-folder reload.
8. History remains bounded, expires consistently with the existing action Undo window, and uses understandable action labels without exposing queue identifiers.

## Contract and storage decisions

- Use the existing `mail_action_operations` receipt and Undo ledger, with dedicated action kinds for Reply Later membership, Read Later membership, Snooze, and Unsnooze (or a single validated triage kind plus explicit operation). Do not create a second local or helper Undo history.
- Add a versioned typed triage mutation request and response. The request identifies the queue operation, account-qualified conversation, desired membership or wake deadline, and requires `Idempotency-Key`. The response contains `changed`, helper `operationId`, confirmed/unknown state, expiry, and a content-free affected queue reference.
- Snapshot prior and expected state before the provider write. For Snooze, capture an optional previous RFC3339Nano `wakeAt` plus expected deadline; distinguish absent from present. Validate wake deadlines as future at mutation time.
- Add typed triage refs to durable action events and mutation/Undo responses. Keep event payloads account-qualified and content-free. The desktop adapter translates them to the existing Reply Later, Read Later, and Snooze Wails events; cursor replay produces the same targeted refresh.
- Helper `mail.write` remains the mutation and Undo capability; bounded history remains under `mail.read`. Do not add a new capability.
- Preserve local-only behavior as a fallback only when the configured local helper is actually unavailable. In remote mode, no helper DB, triage store, provider client, or local Undo stack is constructed or consulted.

## Dependency-ordered implementation slices

1. **HOSTED-SERVER-065 — durable helper triage mutations:** add validated request/response types, receipt reservation/replay, queue snapshots and helper mutation adapters for the three queue stores. Include provider-label side effects in the operation boundary. Preserve existing list/count endpoints.
2. **HOSTED-SERVER-066 — triage Undo execution and event feed:** implement compare-before-restore inverses through existing `/v1/undo/*`; add typed affected queue refs to action/Undo receipts and durable cursor events; keep unknown outcomes non-retryable.
3. **HOSTED-SERVER-067 — desktop adapter and Undo UX:** return helper operation IDs to Wails; route Reply Later/Read Later/Snooze/Unsnooze actions and operation-scoped Undo through the helper in local and remote modes; remove local-store prerequisites in remote mode; emit immediate and replayed queue reconciliation events and map triage action labels in history/toasts.
4. **HOSTED-SERVER-068 — cutover audit:** verify every frontend queue mutation call path and ensure no successful helper mutation is duplicated into local state or local Undo. Gate unsupported queue mutation flows explicitly if any path cannot yet use the helper contract.

## Acceptance criteria

- A changed queue operation returns one durable helper operation ID and appears in bounded Undo history after helper restart.
- Repeating a successful mutation with the same receipt key returns the original receipt without repeating provider or SQLite effects; a changed payload conflicts. A repeated state-setting request with a new key is a no-op with no extra Undo item.
- Reply Later/Read Later Undo restores queue membership and the associated Gmail label; provider ambiguity returns/retains `outcome_unknown` and is not presented as confirmed success.
- Snooze replacement Undo restores the previous deadline; Snooze Undo removes a newly created deadline; Unsnooze Undo restores the removed deadline. A changed/expired entry fails the compare guard without overwriting newer state.
- Local-helper and remote-desktop Wails flows expose the same helper-owned history and Undo behavior. Remote mode works without a local mailbox DB/store and never falls back on auth, capability, or API errors.
- Open queue views refresh from typed affected refs immediately and after event-cursor replay; unrelated folders are not fully reloaded.
- Undo history exposes only safe action labels, timestamps, expiry, count/state and operation ID; no account/conversation refs or mailbox content appear in history summaries.
- Local fallback remains behaviorally unchanged only when the local helper is unavailable.

## Deferred / explicit boundary

- This design covers personal triage queues, not mail read/unread/star/archive/move actions already represented by generic message action snapshots.
- Cross-account provider grouping is not relevant to one account-qualified queue reference, but a future bulk queue API must add partial-group receipts before accepting multi-account batches.
- Live Gmail label behavior and native macOS toast/menu behavior require later target-device verification.

## Status

Design only; no runtime changes. Implement in HOSTED-SERVER-065 through HOSTED-SERVER-068 after review. This closes the mismatch recorded in `docs/plans/2026-11-08-remote-desktop-startup.md`: undo operation references must be helper-owned and service-safe, while device-local composer editing Undo remains in the frontend.
