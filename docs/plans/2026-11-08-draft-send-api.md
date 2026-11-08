# Draft and Send API Implementation Plan

**Goal:** Expose helper-owned draft persistence and explicit message sending through the authenticated v1 API, with revision checks and durable idempotency for delivery-sensitive operations.

**Architecture:** Reuse the existing compose, draft, SMTP, account, and idempotency services rather than duplicating delivery logic. Draft endpoints carry compose fields and expected revisions; send requires an idempotency key and stores a durable outcome receipt so retries can replay accepted or unknown results without initiating a second SMTP attempt. Keep message content in the helper-owned database and never imply recipient delivery from SMTP acceptance.

**Tech Stack:** Go, `internal/automation/v1`, `app/draft.go`, `app/compose.go`, `internal/operations/mail.go`, existing SQLite receipt patterns.

---

### Task 1: Map reusable compose and delivery contracts

1. Identify the shared compose operation inputs, draft model/revision fields, identity checks, encryption/signing hooks, and SMTP result states.
2. Reuse the existing operation/idempotency receipt abstraction where it can represent accepted, rejected, and `outcome_unknown` states without resending.
3. Define bounded request sizes and account/folder ownership validation for draft and send requests.

### Task 2: Implement helper draft operations

1. Add v1 handlers for draft create/read/update/delete and reply preparation using existing compose services.
2. Require the relevant draft read/write and mail send capabilities; reject stale draft revisions before mutation.
3. Add client methods used later by the detached composer, keeping response shapes free of account secrets.

### Task 3: Implement safe send receipts

1. Add an explicit send operation using the shared SMTP/Gmail delivery path.
2. Persist the idempotency key, request fingerprint, operation ID, and terminal/uncertain outcome before replay can occur.
3. Once transport may have accepted a message, never issue another send for the same key; replay the recorded state and expose uncertain outcomes for reconciliation.

### Task 4: Document and verify

1. Document routes, capabilities, revisions, receipt states, and SMTP acceptance semantics.
2. Run `gofmt`, focused Go builds, and `git diff --check`; tests are omitted under the session's instruction.
3. Review account/sender/recipient scope checks and retry paths, update `TASKS.md`/`CHANGELOG.md`, commit, and push the branch.
