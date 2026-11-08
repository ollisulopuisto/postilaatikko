# HOSTED-SERVER-066 Triage Event Replay Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Recover queue changes after a desktop disconnect by including content-free account-qualified triage refs in the helper's existing durable action event cursor.

**Architecture:** Extend the current action-event schema with a separate typed triage-ref JSON column, preserving existing message-ref data and cursors. Persist the triage ref atomically with a triage action reservation so a helper restart can emit a targeted `outcome_unknown` event. Mutation and Undo completion write typed triage refs into the same transaction as the event. The authenticated cursor routes remain unchanged; the desktop dispatches queue state when known and a queue-specific invalidation when the outcome is unknown.

**Tech Stack:** Go, SQLite migration 73, `internal/automation/v1`, `internal/alwayson/action_receipt_ledger.go`, app Wails reconciliation events, frontend queue stores.

---

## Implementation steps

1. Add `affected_triage_json` to `mail_action_operations` and `mail_action_events`; never overload existing message JSON.
2. Add a typed `AffectedTriageRef` to mutation, Undo and event payloads, with queue kind, account/conversation identity, state-known marker, desired membership and Snooze deadline.
3. Add a triage-specific atomic `ReserveTriage` operation that records the ref in the receipt transaction before provider effects. Use it for the existing triage handler; normal message action reservation remains unchanged.
4. Write typed triage refs on confirmed mutation completion, Undo completion, and startup recovery of interrupted triage mutations/undos. Keep events content-free and use existing 90-day retention, per-client cursors and bounded pages.
5. Dispatch confirmed state through existing queue change events; dispatch unknown outcomes as targeted queue invalidations so the client reloads only the affected queue. Preserve ack-after-dispatch semantics.
6. Run `gofmt`, `git diff --check`, and focused app/helper build. Do not add or run automated tests under the current session instruction.

## Invariants

- A lost mutation response or helper restart cannot lose the affected account/conversation identity.
- Event cursors remain account-independent and client-scoped as today; authorization is still `mail.read`.
- Queue refs contain no message content, headers, provider credentials or raw mutation keys.
- Existing message `affected` payloads and cursors remain backwards compatible; old rows with no triage JSON decode as an empty list.
- Unknown outcomes trigger a queue-specific refresh, never optimistic state assignment.
- Event acknowledgment remains after dispatch, so an interrupted client can replay at least once.
