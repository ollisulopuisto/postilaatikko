# Account Service API Design Plan

**Goal:** Specify a safe ownership boundary for account setup, account configuration, and provider credentials before moving those writes out of the desktop.

**Architecture:** Audit current Wails account operations, OAuth callback/token persistence, credential encryption/keyring behavior, helper runtime dependencies, and detached composer. Define authenticated API operations and their capability, request/response, replay, rollback, and lifecycle contracts. The helper remains the only writer of account and provider credential state after cutover; secret material is accepted only by narrow mutation operations and is never returned by account/catalog reads.

**Tech Stack:** Go, `net/http`, account and credential stores, helper OAuth transport, desktop mailbox-service connector.

## Task 1: Claim and audit current behavior

**Files:**
- Modify: `TASKS.md`
- Create: `docs/plans/2026-11-08-account-service-api-design.md`
- Inspect: `app/account.go`, `app/oauth.go`, `app/oauth_creds.go`, `internal/account`, `internal/credentials`, helper transport/runtime composition, detached composer.

**Steps:**
1. Claim HOSTED-SERVER-033 before analysis.
2. Map each account, password, OAuth, custom-provider, shared-mailbox, enable/order, removal, and lifecycle operation to its current store and side effects.
3. Identify how to keep helper sync and SMTP transports current after account or credential changes.
4. Commit the claim and plan before continuing the audit.

## Task 2: Specify the service contract

**Files:**
- Modify: `docs/plans/2026-11-08-account-service-api-design.md`
- Modify: `docs/plans/2026-11-08-service-database-ownership-protocol.md`

**Steps:**
1. Specify account-management authorization separately from ordinary mailbox writes and describe desktop consent/revocation behavior.
2. Define account list/detail and create/update/remove/enable/order operations, credential handoff for IMAP/SMTP passwords and OAuth tokens, custom OAuth configuration, and shared mailbox dependencies.
3. Define secret non-disclosure, TLS requirements for remote endpoints, idempotent retries, cross-store rollback/recovery, and helper cache/sync lifecycle effects.
4. Split implementation into dependency-ordered tasks with explicit acceptance criteria. This task makes no runtime changes.

### Findings

- `app/account.go` performs account create/update/delete, enable/disable, ordering, shared-mailbox creation, credential writes, and local IDLE/pool lifecycle changes. `AddAccount` and OAuth setup create the account row before writing credentials and delete the row if credential persistence fails. `UpdateAccount` snapshots passwords and restores them if a later credential or account-row update fails. Credential cleanup during removal is currently best-effort before deleting the account row; the service should preserve account data when cleanup fails and surface a repairable result rather than report a clean removal.
- Passwords and OAuth material are stored by `internal/credentials.Store`, which prefers the service host's OS keyring and otherwise encrypts values in the service database. OAuth tokens and per-account custom OAuth provider definitions include refresh tokens/client secrets. They must only enter through authenticated mutation bodies over the local loopback transport or a future TLS-protected remote transport; they must never be included in account, status, error, or audit responses/logs.
- OAuth authorization currently completes in the desktop because the app owns browser callbacks and pending in-memory tokens. The desktop can remain the OAuth user-agent: after callback validation it sends the account configuration and token package once to the helper, which stores tokens and custom-provider configuration. Existing-account reauthorization must replace tokens on the service and propagate Microsoft token refreshes to linked shared-mailbox records. Pending tokens should be cleared only after service acknowledgement.
- `mail.write` already covers mailbox mutations such as flagging and moving messages. Account setup can add or remove all mailbox access and secrets, so account operations require a separate `account.manage` capability. Existing grants must not gain it implicitly. The desktop should require an explicit local consent/repair action to register or upgrade its first-party grant, display account-management scope separately, retain the token only in the OS keyring, and support revocation through existing grant administration. The account capability should be visible through the current capabilities endpoint and denied independently at every route.
- Current API client provisioning is a local helper CLI over the shared database. The desktop connector currently provisions a read/body-fetch-only grant automatically and treats missing required capabilities as an authorization error. A capability upgrade must be a deliberate consent action, preserve the old grant until the replacement token is safely stored, then revoke the old grant; failed keychain writes must revoke the newly issued grant.
- Helper `SyncRuntime` owns the account store, sync engine, scheduler, and IMAP pool. `imap.Pool.CloseAccount` can invalidate provider connections after configuration or credential changes. Enable/disable and sync interval changes must update/cancel scheduler state and trigger a bounded refresh as appropriate; account deletion must stop sync, close pool connections, remove credentials, then delete the account and its cascaded data. A failed credential delete must leave the account record recoverable. The helper's SMTP transport also owns separate account/credential handles and will need a shared lifecycle hook or reopen strategy after changes.
- Microsoft shared mailbox creation validates access using the primary account's OAuth credential, creates a linked child account, and stores a credential copy for that child. Deleting a parent cascades to children. Token replacement must update linked children consistently, and child creation must be idempotent or return the existing linked account on retry.
- Detached compose opens `aerion.db` and constructs account, draft, credential, folder, message and security stores. It also refreshes OAuth tokens and sends mail locally. Moving account setup alone does not finish the split: detached compose must be migrated to service draft/send and account-read operations, while device-local contacts and signing/encryption key UX need their own ownership decision.

### Proposed API contract

All routes use the existing `/v1` authenticated API and request-ID/error envelope. The account-management routes require `account.manage`; ordinary mailbox writes remain `mail.write`.

| Operation | Request/response contract | Side effects and retry behavior |
| --- | --- | --- |
| `GET /v1/accounts` and `GET /v1/accounts/{id}` | Return account configuration needed by settings/composer, stable IDs, enabled/order/shared-parent state and sync preferences. Omit passwords, OAuth tokens, custom OAuth client secret, keyring/fallback details and device key material. | Read-only, account-scoped; unknown IDs return not found. |
| `POST /v1/accounts` | Accept non-secret account configuration plus an optional one-time credential envelope. Return only the resulting non-secret account representation and operation ID. | Require `Idempotency-Key`; same key and same request returns the prior result; same key with a different body is conflict. Validate configuration before writes; create row then save credentials; compensate row creation on credential failure. |
| `PUT /v1/accounts/{id}` | Accept mutable non-secret configuration and explicitly present credential actions (`keep`, `replace`, or `clear`) so omitted secrets cannot erase or accidentally replace credentials. | Require idempotency key. Snapshot current credential state; update secret storage and row; restore snapshots on failure. If restoration fails, return a stable partial-failure code and operation ID for repair; do not log secret values. |
| `POST /v1/accounts/{id}/credentials` | Accept password/SMTP password replacement or clear operations, or OAuth token replacement with provider/scopes/expiry metadata. Response contains only accepted/updated status and operation ID. | TLS required off-loopback. Never expose a read-back or credential-presence oracle beyond a coarse configured boolean if the UI needs it. Invalidate account IMAP connections and refresh helper transport state after commit. |
| `POST /v1/accounts/{id}/oauth` | Accept validated OAuth callback result, provider, access/refresh tokens, expiry, granted scopes, stable provider identity, and (for custom OAuth only) the provider definition. No client secret in query strings or URL logs. | One-time idempotency key; helper validates account/provider compatibility, stores tokens and custom provider config, propagates supported shared-mailbox credentials, then acknowledges. Desktop clears pending callback material only after acknowledgement. |
| `DELETE /v1/accounts/{id}` | Return completed/deleted operation state only. | Require idempotency key. Stop scheduler/IDLE work, close pool connections, delete secrets, then delete account data. If secret cleanup fails, keep the account row and return repairable failure. Parent removal applies the same sequence to children and reports partial completion. |
| `PATCH /v1/accounts/{id}/enabled` and `PUT /v1/accounts/order` | Return updated non-secret account state or order. | Idempotent set operations; update scheduler and helper lifecycle before acknowledging. |
| `POST /v1/accounts/shared-mailboxes` | Accept primary account ID and shared mailbox identity/display fields; return non-secret child account. | Validate through the primary account's helper-owned OAuth credential. Require idempotency key and avoid duplicate children. |

Use an opaque operation/idempotency table in the service database to survive helper restarts and retries. Do not persist incoming secrets in that table or include request bodies in diagnostics. Store a keyed/digested request fingerprint only for conflict detection. Keyring and SQLite are not one atomic transaction: implement ordered writes with credential snapshots/compensation where available, preserve the account row on failed deletion, and return explicit repair state if compensation also fails. Provider network validation happens before durable writes where practical; avoid holding a DB transaction across network calls.

The helper's existing listener is loopback HTTP and is protected by bearer grants plus origin restrictions. That is acceptable only for local IPC on the same host. Any non-loopback listener must require HTTPS with certificate validation (or an explicitly configured secure tunnel); bearer tokens and OAuth/password bodies must never traverse plaintext remote HTTP. Body-size limits, request deadlines, and redaction apply to all secret-bearing routes.

### Dependency-ordered implementation tasks

1. **HOSTED-SERVER-034 — Account API capability and safe desktop grant upgrade.** Add/validate `account.manage`, expose it in capabilities and CLI grant management, and add an explicit desktop consent/repair flow that atomically replaces the stored grant and revokes the prior one only after keychain persistence. Acceptance: existing grants do not acquire the capability; denied account routes return the normal capability error; failed keychain persistence leaves the prior desktop grant usable.
2. **HOSTED-SERVER-035 — Helper account catalog, CRUD and lifecycle.** Add secret-free account list/detail/create/update/delete/enable/order and shared-mailbox operations to `SyncRuntime`; own scheduler and IMAP pool refresh/close effects, and include idempotency plus recoverable deletion semantics. Acceptance: helper is the sole writer through these API operations; retries do not duplicate accounts; failed credential deletion preserves account data; account reads never contain credential fields.
3. **HOSTED-SERVER-036 — Password and SMTP credential handoff.** Route account create/update credential envelopes into helper `credentials.Store`, preserve snapshot/restore behavior and distinguish keep/replace/clear. Acceptance: credentials are not stored in API logs/responses or SQLite operation records; failed multi-store updates restore prior state or surface repairable partial failure; helper IMAP and SMTP consumers use the new values after acknowledgement.
4. **HOSTED-SERVER-037 — OAuth onboarding, reauthorization, custom providers and shared mailbox tokens.** Keep browser callback handling in desktop initially and hand callback tokens to helper over authenticated transport; move token/custom-provider persistence and refresh propagation to helper. Acceptance: desktop clears pending secrets only after helper acknowledgement; refresh and reauthorization work after desktop shutdown; custom client secrets never appear in reads; linked Microsoft mailboxes receive consistent replacement tokens.
5. **HOSTED-SERVER-038 — Remove detached-composer account/credential database ownership.** Route account reads and draft/send/token-refresh operations through service APIs; identify and retain device-local contact and signing-key flows separately. Acceptance: detached composer no longer opens service DB or credential store; send/draft failure and reconciliation retain current user data and delivery safety.

HOSTED-SERVER-034 precedes 035 and 036; 035 and 036 precede 037; 038 depends on service account reads and draft/send API readiness. The account API tasks do not by themselves complete database separation; remaining direct desktop service-store consumers must be inventoried and removed before the device-local database cutover.

## Task 3: Review

1. Verify the design statements against current code and existing API capability/revocation behavior.
2. Run `git diff --check`; do not run tests because this task changes documentation only.
3. Update the task ledger with the design commit and remaining implementation sequence, then commit and push for coordinator integration.
