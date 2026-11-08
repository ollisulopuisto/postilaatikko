# HOSTED-SERVER-055: Persistent Action Receipts

## Goal

Make the always-on helper's mail action idempotency durable across helper restarts. The operation intent must be written before provider effects. A repeated key with a different request conflicts; a matching terminal request returns its stored outcome; a matching intent interrupted by process death becomes `outcome_unknown` and is never replayed automatically.

## Scope

- Add a service database migration for action operation receipts.
- Add a small receipt-store contract to `internal/automation/v1` and a SQLite implementation in `internal/alwayson`.
- Hash the caller idempotency key before persistence; persist the canonical request hash, operation ID, status, response, and timestamps only.
- Integrate the durable store into the helper action handler while retaining an in-memory default for isolated handler callers/tests.
- Recover unfinished rows as `outcome_unknown`, and expose a safe lookup path by idempotency key for the authenticated caller.
- Add focused tests for restart replay, conflicting requests, interrupted operations, and concurrent duplicate submissions.

## Boundaries

- Provider-group checkpoints and cross-account reconciliation are follow-up work; this slice must never automatically repeat an action after an ambiguous failure.
- Action-specific prior-state snapshots and Undo execution belong to HOSTED-SERVER-056.
- Do not persist raw keys, bearer credentials, message headers, bodies, or attachment data.
- Keep operation receipt lookup under `mail.write` for now, matching mutation authorization.

## Acceptance

1. A successful action replayed after handler/runtime reconstruction returns the original operation ID and response without another provider call.
2. The same key with a changed action, target, or destination returns conflict.
3. A previously reserved but incomplete operation is returned as `outcome_unknown`, with no provider call on retry.
4. Concurrent submissions using the same key execute provider work at most once.
5. Database rows contain only a one-way key digest, request fingerprint, safe receipt data, and timestamps.
6. Focused Go tests, helper/app builds as appropriate, and `git diff --check` pass.

## Implementation sequence

1. Add migration and SQLite ledger with atomic reserve/read/complete behavior.
2. Introduce the handler store interface and durable operation recovery/replay flow.
3. Wire the always-on runtime's database and fingerprint provider into the action handler.
4. Add restart, conflict, uncertain, and concurrency regressions.
5. Record exact verification and remaining limitations in `TASKS.md`.
