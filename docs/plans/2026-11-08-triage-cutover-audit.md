# HOSTED-SERVER-068 Triage Cutover Audit

**Goal:** Audit every frontend and Wails mutation route for Reply Later, Read Later, Snooze, and Unsnooze after the helper-owned receipt cutover, then repair any duplicate write, desktop-memory Undo, unsafe remote fallback, or unclear unsupported path.

## Audit sequence

1. Enumerate exported Wails mutation methods, frontend imports/call sites, keyboard/context-menu actions, background expiration, and Undo callbacks for all four triage queues.
2. Trace each path through helper availability, authorization, idempotency, mutation, immediate event emission, cursor replay, and Undo dispatch.
3. Confirm helper-success paths only update the desktop presentation cache, never write a second local queue/provider mutation or add a memory Undo entry.
4. Confirm remote disconnected, unauthorized, and API error paths fail closed and never reach local mailbox stores/providers. Check background Snooze expiration for remote mode.
5. Fix gaps found and record paths plus remaining limitations in this plan and `TASKS.md`.
6. Run `gofmt`, `git diff --check`, and focused app/helper build only. Do not add or run tests under the current session instruction.

## Invariants

- Helper-owned queue state and Undo receipts are canonical whenever helper mode is active.
- Frontend cache updates and event handlers are presentation reconciliation, not second backend mutations.
- Remote failures never fall through to local database or provider stores.
- Any supported local-only fallback is explicit and does not claim a helper Undo receipt.

## Findings

- Primary-window Read Later, Snooze, Unsnooze, and Reply Later call the result-bearing app methods. Helper success emits presentation reconciliation and Undo-history events; the frontend store only updates its cache. No helper path also writes `triageStore`/`replyLaterStore` or pushes a desktop Undo command.
- The detached reader and mail-window method allowlists did not expose the new Read Later/Snooze receipt methods or the Read Later queue read. Added those methods to the shared reader allowlist so context-menu and `L` shortcut actions work in secondary windows.
- Reply Later's older connector availability check treated every `service_unavailable` result as local fallback. Replaced it with the shared service gate, which permits fallback only for a stopped local helper and fails closed for remote endpoints.
- Remote queue reads and Snooze maintenance now consistently use the helper or return an explicit unavailable error. Local-store fallback remains only for local mode when its helper is stopped.
- Legacy boolean/error Wails wrappers remain for compatibility. They delegate to the result-bearing helper-backed operations, so they do not add a second mutation or local Undo entry.
