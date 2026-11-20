# Helper-owned Gmail filter publication

The existing Gmail review dialog now supports the configured account-owning helper and the explicit standalone workspace. No failed helper operation can switch to a desktop rule or credential store.

## Desktop review

The dialog captures the configured connector, endpoint and client identity once. It displays the frozen execution owner and endpoint. Every subsequent operation supplies that workspace key. Changing the connector, endpoint or credential client invalidates the open review, even if the next workspace has the same account or rule IDs. Reopen the dialog to select a new owner.

Read-only context and status discovery resolve the selected source account on the helper. No desktop account lookup supplies a helper account. Preparation, publication, cancellation and reconciliation remain explicit requests with selected account, source and frozen plan identity. Provider work has a bounded 30-second request timeout, not the connector's shorter health-read timeout. No create or consent completion is retried.

## Consent

The desktop authorization button starts an IPv4 loopback listener. The helper retains the OAuth project, PKCE verifier and state, exchanges the forwarded callback code once, verifies the live Gmail identity and all required scopes, and persists its canonical Mail grant. No refresh token, project secret or verifier crosses the API.

Sessions expire after five minutes, are bound to the authenticated API client and selected account, and are cancelled on unsuccessful desktop exit. The desktop opens only the expected Google authorization origin with its own listener redirect. A workspace change before callback forwarding blocks completion. If the completion response is lost, reopen review to check account context; never replay the code.

Helper completion rechecks expiry, shutdown, account/project binding and the original grant before saving. Credential snapshots use compare-and-swap persistence. Helper Mail refresh paths also refuse stale responses so an older refresh cannot overwrite a newly consented settings grant. The keyring and SQLite are not transactional together; persistence/readback errors remain unverified.

## Publication and recovery

`POST /v1/filters/gmail` requires mail.read. Preparation, publication, cancellation and reconciliation additionally require mail.write and account.manage. Consent lifecycle requests require mail.read and account.manage.

Only stopped single-account sources without folder or move scope are eligible. Native payloads contain an explicit Gmail query and supported label actions. Local substring rules are never translated. Selected user labels come from the account's current catalog. Preview reads one page of at most 51 message IDs, showing at most 50 plus truncation. It never changes historical mail.

The durable journal binds account email/OAuth project, source snapshot and payload fingerprint. Provider operations verify Gmail profile and pin the same token for the following request. Preparation rejects outstanding automatic jobs and locks the source. An unknown create never replays. Status reload and read-only reconciliation recover it. Only an unattempted plan can cancel. Published sources stay guarded; reverse transfer and provider deletion are not implemented. Old unbound plans fail closed.

## Validation boundaries

Tests use synthetic helper credentials and mocked provider HTTP. Desktop tests cover workspace/client replacement, offline helper refusal, no local fallback, exact draft routing, a single failed create, a real loopback callback forwarding sequence and rejection of unexpected authorization destinations. Helper tests cover source-owned account/status discovery, permissions, PKCE, grant changes, expiry/shutdown during exchange, journal recovery and refresh races. The actual browser test covers selected helper consent, preview failure/invalidation, explicit final publication, uncertain-create recovery, reopen and EN/FI narrow layouts.

No real Studio, live Google consent or mail matching, or native macOS acceptance has been run. Required CI and exact-head review still gate every merge. No build or release dispatch is part of this change.
