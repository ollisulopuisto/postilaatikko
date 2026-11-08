# Account Management Capability Plan

**Goal:** Add a separately granted `account.manage` capability and an explicit desktop authorization path without weakening existing mailbox-service grants.

**Architecture:** Extend the versioned v1 capability registry and local API-client administration. Generalize desktop grant replacement to add a requested capability while retaining existing capabilities. Require an explicit settings action for account-management scope, persist the replacement bearer in the OS keyring before revoking the prior grant, and expose account-management authorization separately from `mail.write` status.

## Task 1: Claim and inspect

1. Claim HOSTED-SERVER-034 from latest main in the isolated worktree.
2. Verify current capability validation, CLI client grant management, desktop connector provisioning, `mail.write` upgrade, and settings UI.
3. Commit the claim and plan before implementation.

## Task 2: Implement capability and consent

1. Add `account.manage` to the known v1 capability allowlist and CLI help.
2. Preserve every existing desktop capability during grant upgrades, add only the explicitly requested capability, persist new keyring state before revoking the old grant, and retain a usable credential on rollback failure.
3. Add a distinct settings status and explicit action with copy that explains account setup/credential authority separately from mailbox writes.
4. Keep `mail.write` authorization behavior and account-management authorization independent.

## Task 3: Review

1. Check source for missing capability validation and stale-grant failure paths.
2. Run `git diff --check` and relevant non-test build/check commands only; no tests unless explicitly requested.
3. Record the implementation/review commit in TASKS.md and push the task branch for coordinator integration.
