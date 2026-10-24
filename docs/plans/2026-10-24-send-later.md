# E2EE Send Later Implementation Plan

**Goal:** Send scheduled mail from the always-on Mac Studio while keeping queued content unreadable to any relay.

**Architecture:** Client and helper identities are paired through a local, authenticated setup flow. Clients use the existing Go NaCl box implementation to authenticate and encrypt immutable delivery instructions to the helper. A helper-owned SQLite queue stores only encrypted payloads, atomically claims due jobs, and stops automatic retries when delivery may already have occurred.

**Tech stack:** Go, `golang.org/x/crypto/nacl/box`, OS keyring, SQLite (`modernc.org/sqlite`), existing SMTP/account/OAuth infrastructure.

## Slice 1 — encrypted queue and worker foundation (this branch)

1. Add `internal/scheduledsend/protocol.go`: versioned schedule/cancel commands, bounded payloads, authenticated encryption, stable job/revision IDs, and immutable RFC822 validation.
2. Add `internal/scheduledsend/identity.go`: OS-keyring identity persistence with no plaintext fallback. Key pairing must be verified locally; a relay cannot supply a trusted replacement key.
3. Add `internal/scheduledsend/store.go`: separate helper database, local pairing permissions, ciphertext-only payload persistence, idempotent submission, authenticated cancellation tombstones, atomic due-job claims, and durable result states.
4. Add `internal/scheduledsend/worker.go`: one bounded delivery at a time; classify safe retries, permanent failures, and ambiguous outcomes. Interrupted sends become `outcome_unknown`, never blind retries.
5. Document the threat model, state transitions, metadata leakage, memory bounds, and interfaces needed by the remaining slices.
6. Format and inspect the changes. Tests are deliberately not added/run in this turn under the coordinator's instruction; cryptographic and queue regression coverage remains required before feature activation.

## Slice 2 — headless SMTP delivery

**Files:** `app/compose.go`, new `internal/scheduledsend/smtp_transport.go`, relevant account/OAuth/signing adapters.

1. Extract message preparation from transport and post-send UI/draft cleanup; preserve identities, signatures, S/MIME/PGP, envelope Bcc, and Sent handling.
2. Serialize MIME once before enqueueing. Persist its Message-ID and bytes across retries. Never call current `ComposeMessage.ToRFC822()` again for a queued job, since it generates a new ID/date.
3. Build the helper transport from helper-local account settings and credentials. OAuth refresh remains local. Bind paired clients to explicitly allowed local account IDs and sender addresses.
4. Add structured SMTP delivery outcomes. A failure during/after DATA may mean acceptance is unknown; a stable Message-ID alone does not guarantee deduplication.
5. Keep a Sent-append failure distinct from delivery failure so it never causes a second send.

## Slice 3 — helper process and secure submission

**Files:** new `cmd/postilaatikko-helper/main.go`, helper configuration/install code, client submission API.

1. Add a single-user macOS helper executable and launchd installation/status controls. Explicitly handle locked keychain after reboot/logout.
2. Add verified client/helper key pairing, revocation, permission editing, and durable acknowledgements. Pairing must not trust a key obtained only from an untrusted relay.
3. Add an outbound polling/streaming relay adapter carrying encrypted envelopes. Bound requests and per-user storage; keep private keys, account tokens, content, recipients, and attachments out of logs.
4. Authenticate status/acknowledgement responses end to end before a remote client reports “scheduled on Studio.” Encrypt status content too when it leaves the helper.
5. Define relay deletion/retention and device key rotation. Revocation prevents future decrypt/send authorization; queued payload retention is an explicit user action.

## Slice 4 — compose and scheduled-message UI

**Files:** `frontend/src/lib/components/composer/Composer.svelte`, new schedule picker/list components, app APIs/bindings.

1. Add schedule picker with timezone and local preview, and wait for durable helper acceptance before closing a draft.
2. Add encrypted schedule, revision-based reschedule, cancel, and list/status operations. Cancellation after claim must report that delivery has started, not pretend to recall mail.
3. Add reauthorization/offline/late/unknown outcome views, with manual reconciliation for ambiguous SMTP acceptance.
4. Preserve independent detached composers. Keep queued attachments out of long-lived base64 UI state.

## Required verification before activation

Protocol tampering, unknown/revoked keys, helper key mismatch, replay and out-of-order cancellation, duplicate revisions, permission scope, queue restart and concurrent claims, cancellation races, lost SMTP acknowledgement, transient failures, Sent append failure, expired OAuth credentials, large attachments, and keychain lock/reboot. Runtime activation and release require this coverage and security review; the foundation alone is not a usable Send Later feature.
