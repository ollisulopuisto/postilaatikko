# Gmail-native publication recovery core

This change is a backend prerequisite, not a finished Publish to Gmail feature. No desktop or helper API calls this core yet.

## Reviewed input, not a translation

`PrepareGmailPublication` accepts a provider-native Gmail filter, an account-qualified source rule, the freshly reviewed provider catalog, and the selected account's allowed label IDs. It does not convert local literal substrings into Gmail search syntax. Those matching models and their execution order differ.

The initial actions are label-only. Forwarding, unknown labels, opposing add/remove actions, folder-scoped source rules, local move actions and multi-account sources are refused. The provider draft has a 16 KiB limit. An existing identical Gmail rule stops publication for review instead of creating another.

## Execution ownership

Preparation requires the source rule to be disabled and no durable automatic-filter jobs to remain, including pending, processing or outcome-unknown jobs. Reading the source snapshot and recording the plan take one SQLite transaction and a durable guard. Local edits, reordering, re-enabling, deletion and job enqueue are refused while that guard exists. Only an unattempted plan can be cancelled to release the source.

This is not yet proof that every runtime executor has stopped. Before exposing publication, app/service integration must acquire the account owner's filter-executor quiescence boundary, wait for direct in-flight actions, and then prepare the plan. A desktop must never prepare or publish against unrelated local credentials while its workspace is a remote helper.

## One create attempt

The journal stores the source snapshot, native payload, provider baseline IDs and account. A conditional state change claims `prepared` as `attempted` before create. Competing callers cannot both claim it. No state after that point permits another create.

A successful or uncertain response becomes `unknown` until provider read-back verifies the payload. If a provider ID was returned, only that ID is accepted. Otherwise recovery finds exactly one matching ID absent from the baseline. Zero or multiple matches remain uncertain. Recovery never writes to Gmail, deletes a candidate or retries create. Process death after submission is recoverable from `attempted` in the same way. Account-scoped record IDs can be listed after restart.

## Integration still required

- Explicit opt-in account consent and an account-bound provider adapter.
- Provider-native editor, semantics warning, preview and final review fingerprint.
- Runtime executor quiescence, helper-owned routing and UI recovery states.
- Verified provider-removal flow before any future switch back to local execution.
- Native macOS and live Google acceptance. No live account has been authorized or changed by these tests.

The stored journal contains rule configuration and provider IDs, never access or refresh tokens. Migration 85 is additive and starts with an empty journal.

## Standalone adapter

The app now has internal preparation, publication and recovery orchestration for standalone ownership. An exclusive filter-execution lease drains direct app filtering; delayed callbacks check the journal guard before acting. The provider adapter resolves the selected account's tokens and requires its explicit settings grant. Preparation loads the current provider filter and label catalogs before recording the plan. The source must already be disabled, so preparation does not silently stop an existing rule.

Every mailbox-service connector, including localhost and an offline helper, blocks this local path. A failed health probe is not evidence that an independent executor has stopped. Helper-owned publication must execute on the helper through a guarded account-owner API, which remains to be implemented. The internal methods are not Wails-bound or called by UI until provider-native review is available.
