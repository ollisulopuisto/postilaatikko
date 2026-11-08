# Mail Mutation Reliability

## Goal

Keep desktop and helper state aligned when authenticated mailbox mutations are slow or partially successful across accounts, and verify grant replacement and flag action behavior.

## Scope

- Give mail mutation requests an appropriate timeout and reuse an idempotency key after an ambiguous response.
- Persist provider-confirmed flag updates per account/folder group before attempting later groups.
- Return enough structured partial outcome information for the desktop to reconcile state and emit existing events/count updates.
- Add focused tests for grant credential replacement, authorization and fallback, and flag state/events including multi-account partial failure.

## Acceptance

- A request that completes beyond the former two-second limit is not reported failed solely due to the short client timeout.
- Repeating an ambiguous request reuses its idempotency key.
- Successful account groups are reflected in the local database and desktop event/count state even when a later account fails.
- Tests cover write grants, keyring replacement/rollback, service-unavailable fallback, flag actions, and partial outcomes.

## Verification

- Focused Go tests for app mutation routing, helper action operations, and API idempotency.
- Relevant package build and `git diff --check`.

## Risks

- Provider operations may partially succeed within a single provider batch before returning an error; provider adapters' actual semantics must be checked before defining partial success boundaries.
- Current action idempotency receipts are in-memory, so retry guarantees last only for the running helper process.
