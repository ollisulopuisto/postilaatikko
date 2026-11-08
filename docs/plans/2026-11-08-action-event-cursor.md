# Helper Action Event Cursor Implementation Plan

> **For Claude: REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.**

**Goal:** Let an authenticated desktop recover bounded helper action and Undo events missed while disconnected, using a durable cursor scoped to its API client grant.

**Architecture:** Persist content-free action events in the helper database in the same transaction that completes the action or Undo receipt. Expose bounded page and acknowledgment routes keyed by the authenticated client ID; the desktop polls on startup, window focus, and a slow interval, emits the existing targeted reconciliation event, then acknowledges the delivered sequence for at-least-once replay.

**Tech Stack:** Go, SQLite schema migrations, versioned HTTP API/client, Wails runtime events, Svelte.

---

### Task 1: Persist bounded action events and cursors

**Files:**
- Modify: `internal/database/migrations.go`.
- Modify: `internal/alwayson/action_receipt_ledger.go`.
- Add: event-page types and store contract in `internal/automation/v1/action_events.go`.

**Steps:**
1. Add append-only event and per-client acknowledged-cursor tables with unique `(operationId,state)` events.
2. Insert action events in the receipt completion transaction using only operation ID, terminal state, account/message/folder refs, and creation time.
3. Insert Undo events in the Undo receipt transaction; derive affected refs from the safe Undo snapshot.
4. Convert interrupted operations to durable `outcome_unknown` events during helper recovery.
5. Retain a bounded age window; when a client cursor predates retained events, return `resetRequired` rather than an unbounded scan.

### Task 2: Add authenticated event page and acknowledgment routes

**Files:**
- Add: `internal/automation/v1/action_events.go`.
- Modify: `internal/alwayson/catalog_api.go`.
- Modify: `internal/automation/client/client.go`.

**Steps:**
1. Use the authenticated grant's stable client ID as the cursor owner; do not accept a caller-supplied owner ID.
2. Add a read-capability bounded GET page and an authenticated acknowledgment endpoint.
3. Reject invalid limits, negative/regressing cursors, and acknowledgments beyond the latest stored sequence.
4. Add client methods for fetching and acknowledging event pages.

### Task 3: Recover missed events in the desktop

**Files:**
- Modify: `app/mailbox_service.go` and `app/undo.go`.
- Modify: `frontend/src/App.svelte` and `frontend/src/lib/components/list/MessageList.svelte`.
- Update: `frontend/wailsjs/go/app/App.js` and `App.d.ts` if generated bindings are not regenerated.

**Steps:**
1. Expose a Wails sync method that fetches one bounded page, emits the existing typed reconciliation event, and acknowledges only after event dispatch.
2. Poll on app mount, window focus, and a bounded interval; serialize sync requests.
3. On `resetRequired`, refresh only the currently displayed bounded page and helper-backed counts.
4. Refresh Undo history when replay delivers an uncertain action/Undo state.
5. Run `git diff --check` and relevant Go builds; frontend static checks are conditional on dependencies being installed. No tests are added or run in this session.

---

## Limits

The cursor belongs to an authenticated API client grant, which is the available stable device identity. Event delivery is at least once: clients may receive a duplicate if they fail between emitting and acknowledging. Event rows contain no message bodies, headers, subjects, or paths. Group-level partial provider recovery is tracked separately.
