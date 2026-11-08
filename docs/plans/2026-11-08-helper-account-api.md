# Helper Account API Plan

**Goal:** Make the always-on helper the owner of account catalog and non-secret account configuration changes.

**Architecture:** Add account read and mutation operations to the versioned authenticated API. Read DTOs must be separate from persistence/configuration structs and omit all credentials. Mutation handlers require `account.manage` and call the existing helper-owned account store and sync lifecycle. This task excludes password, OAuth, and custom-provider secret handoff, which follow in HOSTED-SERVER-036/-037.

## Task 1: Claim and map lifecycle

1. Claim HOSTED-SERVER-035 on latest main in its own worktree.
2. Inspect `account.Account`, `AccountConfig`, account store operations, API handler patterns, `SyncRuntime`, scheduler/IMAP pool hooks, helper SMTP transport lifetime, and desktop account call sites.
3. Commit this claim and plan before implementation.

## Task 2: Implement account API and helper lifecycle

1. Define a secret-free account DTO and bounded list/detail API. The initial read slice is implemented: list and detail routes expose account settings without credential fields, and the desktop uses the service when connected.
2. Add account create/update/delete/enable/order/shared-mailbox APIs, guarded by `account.manage`; preserve account validation and expose only non-secret response fields.
3. Make retries safe with an idempotency key and durable operation record or stable operation key; never persist secret request fields. Preserve an account row if credential cleanup is not yet safely handled by the later credential task.
4. Invalidate helper IMAP connections and refresh/cancel sync scheduling when account state changes; ensure SMTP transport resolves current account data/credentials or define a safe reopen hook.
5. Route desktop account catalog/configuration calls through the service where supported, retaining local behavior only when helper is unavailable and propagating authorization/API errors.

## Task 3: Review

1. Check every response and error path for secret fields and account ownership validation.
2. Run focused Go builds and `git diff --check`; do not add or run tests unless explicitly requested.
3. Record implementation and review status, then push the task branch for coordinator integration.
