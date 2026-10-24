# Send Later foundation

This package is the first backend slice of P-011. It provides encrypted commands, helper identity storage, a durable queue, and a transport-independent worker. It is **not yet connected to the composer, a running helper executable, SMTP, or a relay**. Do not advertise Send Later as available or activate a network endpoint from this slice alone.

## Trust and encryption

The client and Mac Studio helper each have a NaCl box identity. Private keys live in the OS keyring under `Postilaatikko.SendLater.v1`; keyring errors stop initialization/unlocking, with no plaintext file fallback. Identity creation is an explicit, serialized local setup operation, separate from loading an existing key. The queue pins the helper public key and refuses to open with a different identity.

An operator must verify the helper/client public keys through an authenticated local pairing exchange. Fetching a helper key from an untrusted relay is not sufficient. `PairClient` grants a particular client explicit `(helper-local account ID, sender address)` pairs. It is a local administrative operation, never a public relay API. The SMTP adapter must also confirm that each sender is a configured identity for the selected helper account.

`SealCommand` uses [`golang.org/x/crypto/nacl/box`](https://pkg.go.dev/golang.org/x/crypto/nacl/box) with a fresh random 24-byte nonce. NaCl box supplies Curve25519/XSalsa20/Poly1305 authenticated encryption; this package does not implement its own cipher. The entire schedule/cancel command is encrypted, including job/revision IDs, account selection, envelope sender/recipients, prepared MIME with attachments, and due time. Public routing keys, ciphertext size, and traffic timing remain observable to a relay. The format is versioned and both sending and receiving reject unsupported versions.

This is E2EE **between the composing device and the trusted helper**. The helper decrypts in memory to deliver the mail using its local credentials. Encryption between the helper and the final email recipient is whatever the mail transport and existing S/MIME/PGP setup provide. Static pairing keys do not provide forward secrecy: compromise of a helper private key can expose captured ciphertext. Rotation, authenticated pairing UX, relay quotas, and authenticated/encrypted remote acknowledgements still need implementation and review before remote activation.

## Queue behavior

`OpenStore` requires a dedicated owner-only directory and uses a separate SQLite database with full synchronization, WAL journaling, one connection, and a modest cache. Only ciphertext payloads are stored; due time, job ID/revision, pairing permissions, and delivery state are helper-local metadata. The relay must never receive this database or plaintext `Status` objects.

Clients must retain and retransmit the **same encoded envelope bytes** when retrying a submission. Exact duplicates return the existing durable status. A different command or fresh nonce with the same revision is a conflict. Higher revisions can update pending/failed jobs; a cancellation arriving before its schedule creates a tombstone so older schedules cannot revive it. Job IDs must be random UUIDs. Do not purge sent/cancelled revision records without an explicit replay-expiration protocol.

`Accept` acknowledges after commit; its local `Status` result is not itself an authenticated remote acknowledgement. A future transport must encrypt and authenticate status/acknowledgement messages before the client reports “scheduled on Studio.”

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

Provider acceptance is the send boundary. Failure to append a Sent copy, update contacts, or clean up a draft after acceptance must never be reported as a retryable delivery failure. Keep those follow-up tasks separately recoverable. Queue status stores fixed error codes, never provider error strings or message content.

Raw MIME is capped at 10 MiB, encoded envelopes at 24 MiB, and ciphertext at 16 MiB for this first slice. Processing is bounded to one payload, but Go/JSON/NaCl still create temporary copies; this is not streaming large attachments or a secure-memory guarantee. Sent payloads are removed from active rows; failed/unknown payloads stay encrypted for reconciliation. Retention cleanup and forward secrecy remain separate work.

## Next integration work

See [the implementation plan](../../docs/plans/2026-10-24-send-later.md) for headless SMTP preparation/transport, helper/launchd lifecycle, pairing and relay transport, composer schedule UI, and required verification. No tests were added/run for this slice under the coordinator's instruction. Compilation and source review do not establish the cryptographic and delivery reliability needed for feature release.
