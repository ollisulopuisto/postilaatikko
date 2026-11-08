# Helper Password Handoff Plan

**Goal:** Make the always-on helper the only writer for account IMAP and SMTP password credentials.

**Scope:** This task covers account-create and account-update password envelopes only. OAuth token/provider persistence remains HOSTED-SERVER-037; detached composer ownership remains HOSTED-SERVER-038.

## Contract and safety

- Extend authenticated `account.manage` create/update operations with explicit `keep`, `replace`, and `clear` actions for the IMAP and SMTP password slots.
- Accept replacement values only in request bodies. Never return them, include them in API errors, log them, or persist request bodies.
- Fingerprint secret-bearing idempotent requests with a keyed digest derived from the helper's device credential key; persist only the digest and public response.
- Capture helper credential snapshots before changes. Apply password and SMTP password changes before committing account configuration; restore snapshots on any failure. If rollback fails, return a stable repair-required error and preserve the account row.
- For a newly created account, remove the row only when password compensation is confirmed; otherwise retain it disabled for repair.
- Refresh helper IMAP pool/scheduler state after successful credential/config acknowledgement. SMTP sends construct configuration from the helper's current account and credential stores for each delivery.
- When the desktop helper is connected, send account password actions through this API and do not write them through the desktop credential store. Keep the existing local path only when the helper is unavailable.

## Implementation sequence

1. Claim HOSTED-SERVER-036 on latest main in a dedicated worktree; commit this plan and claim before code changes.
2. Inspect the current credential snapshots, keyring fallback, account mutation API, desktop account setup/update paths, and helper SMTP/IMAP lifecycle.
3. Add secret-bearing request DTOs and secret-safe HMAC idempotency fingerprints; make helper credential application compensating and ordered with account-store writes.
4. Route desktop create/update password flows through the helper and ensure secret fields are not retained in read DTOs, responses, receipts, logs, or helper diagnostics.
5. Run focused Go builds and `git diff --check`. Do not run tests unless the user asks.
6. Record the commit and known limits in TASKS.md, then push for coordinator integration.
