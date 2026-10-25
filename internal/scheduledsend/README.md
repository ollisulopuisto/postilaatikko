# Send Later foundation

This package provides encrypted commands/receipts, helper identity storage, durable helper and client queues, a transport-independent worker, and a helper-local SMTP adapter. `cmd/postilaatikko-helper` exposes a bounded direct HTTP(S) endpoint and CLI; the main and detached composers have scheduling/setup/status controls. The implementation is a review milestone, not a released or verified Send Later feature. A hosted relay is not implemented. See [setup instructions](../../docs/SEND_LATER.md).

## Trust and encryption

The client and Mac Studio helper each have a NaCl box identity. Private keys live in the OS keyring under `Postilaatikko.SendLater.v1`; keyring errors stop initialization/unlocking, with no plaintext file fallback. Identity creation is an explicit, serialized local setup operation, separate from loading an existing key. The queue pins the helper public key and refuses to open with a different identity.

An operator must verify the helper/client public keys through an authenticated local pairing exchange. Fetching a helper key from an untrusted relay is not sufficient. `PairClient` grants a particular client explicit `(helper-local account ID, sender address)` pairs. It is a local administrative operation, never a public relay API. The SMTP adapter must also confirm that each sender is a configured identity for the selected helper account.

`SealCommand` uses [`golang.org/x/crypto/nacl/box`](https://pkg.go.dev/golang.org/x/crypto/nacl/box) with a fresh random 24-byte nonce. NaCl box supplies Curve25519/XSalsa20/Poly1305 authenticated encryption; this package does not implement its own cipher. The entire schedule/cancel command is encrypted, including job/revision IDs, account selection, envelope sender/recipients, prepared MIME with attachments, and due time. Public routing keys, ciphertext size, and traffic timing remain observable to a relay. The format is versioned and both sending and receiving reject unsupported versions.

This is E2EE **between the composing device and the trusted helper**. The helper decrypts in memory to deliver the mail using its local credentials. Encryption between the helper and the final email recipient is whatever the mail transport and existing S/MIME/PGP setup provide. Static pairing keys do not provide forward secrecy: compromise of a helper private key can expose captured ciphertext. Rotation, relay quotas and deployment remain future work. Public-key comparison is explicit in the setup UI and local helper CLI; no relay may silently replace trusted keys.

## Queue behavior

`OpenStore` requires a dedicated owner-only directory and uses a separate SQLite database with full synchronization, WAL journaling, one connection, and a modest cache. Only ciphertext payloads are stored; due time, job ID/revision, pairing permissions, and delivery state are helper-local metadata. The relay must never receive this database or plaintext `Status` objects.

Clients must retain and retransmit the **same encoded envelope bytes** when retrying a submission. Exact duplicates return the existing durable status. A different command or fresh nonce with the same revision is a conflict. Higher revisions can update pending/failed jobs; a cancellation arriving before its schedule creates a tombstone so older schedules cannot revive it. Job IDs must be random UUIDs. Do not purge sent/cancelled revision records without an explicit replay-expiration protocol.

`Accept` acknowledges after commit; its local `Status` result is not itself an authenticated remote acknowledgement. `Handler` encrypts and authenticates acknowledgements to the paired client and binds each to the exact request hash. Status queries use fresh encrypted requests. The client rejects redirects, wrong helper/client keys, oversized replies and receipts for another request before reporting “scheduled on Studio.” Pairing/revocation/unknown-outcome resolution remain local administrative operations.

State transitions:

```text
queued/retry -> sending -> sent
                       -> retry                 only when non-acceptance is known
                       -> failed                permanent error / authorization / retry limit
                       -> outcome_unknown       ambiguous transport response
queued/retry/failed -> cancelled                 authenticated higher revision
outcome_unknown -> sent/failed                   verified local reconciliation only
```

Claiming is atomic across processes. The worker loads only one due payload at a time and waits for the next due time; local submissions wake it immediately. Another process's submissions are observed within 30 seconds. Due jobs are processed after restart even when their planned time has passed. Claims still in `sending` after 15 minutes are marked `outcome_unknown`, never resent automatically. Transports have a two-minute deadline and must honor it.

Clients cannot cancel/reschedule a job after a worker claims it. `ErrDeliveryStarted` explicitly means that delivery may already be underway. Revocation blocks pending jobs immediately and rechecks authorization before transport; it cannot recall a transport invocation already in progress.

## Delivery contract

The client must serialize/sign/encrypt RFC822 **once before scheduling**, preserving those bytes and a stable Message-ID on every attempt. The existing `ComposeMessage.ToRFC822()` generates a fresh Message-ID and Date per invocation, so it must not be called repeatedly by a retry worker. Envelope Bcc recipients remain in `Delivery.To`; a Bcc header in the MIME payload is rejected.

A `Transport` uses helper-local account credentials. It returns `DeliveryError{Kind: SafeRetry}` only when it knows the mail provider did not accept the message. A dropped connection during/after DATA, timeout with uncertain acceptance, or an untyped error becomes `outcome_unknown`. A stable Message-ID is useful for reconciliation but does not guarantee provider deduplication or exactly-once delivery.

Provider acceptance is the send boundary. Failure to append a Sent copy, update contacts, or clean up a draft after acceptance must never be reported as a retryable delivery failure. The SMTP adapter returns `AcceptedWithoutSentCopy` for the first case; the worker records `sent` with `sent_copy_failed`, retains encrypted payloads for future copy recovery, and never repeats SMTP. The original client draft remains available. Queue status stores fixed error codes, never provider error strings or message content.

Raw MIME is capped at 10 MiB, encoded envelopes at 24 MiB, and ciphertext at 16 MiB for this first slice. Processing is bounded to one delivery and at most two concurrent HTTP requests, but Go/JSON/NaCl still create temporary copies; this is not streaming large attachments or a secure-memory guarantee. Sent payloads without follow-up warnings are removed from active rows; failed/unknown/Sent-copy-warning payloads stay encrypted for reconciliation. Retention cleanup and forward secrecy remain separate work.

## Next integration work

See [the implementation plan](../../docs/plans/2026-10-24-send-later.md) and [helper instructions](../../docs/SEND_LATER.md). Direct helper SMTP/HTTP delivery, login-agent installation, and basic composer scheduling/cancellation/status are implemented for review. Rescheduling UI, dedicated Scheduled view, remote relay transport, key rotation, quotas/retention and live lifecycle/security/reliability verification remain. No tests were added/run under the coordinator's instruction. Compilation and source review do not establish the cryptographic and delivery reliability needed for feature release.
