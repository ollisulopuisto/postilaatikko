# Remote S/MIME and PGP parity: security design for review

Status: proposal only. Existing remote crypto gates stay closed. This document does not authorize a key migration or claim working remote crypto.

## Trust boundary and custody

This work targets an owner-operated, trusted mailbox helper, not the hosted zero-knowledge service. Those are different products: a helper that can decrypt mail or sign as the owner is a trusted security principal.

Keep private keys on their current device by default. Never silently copy private keys, passphrases or credential-store contents when a desktop connects to a remote helper. Public certificates/keys and private-key material require different operations and grants. Sending an ordinary message does not authorize exporting a signing key.

Two implementations can preserve local parity, with different costs:

- Device-local crypto: remote mailbox reads return bounded original MIME; the desktop verifies/decrypts and signs/encrypts using a device-owned crypto store. Only final prepared MIME goes to the helper's existing send path. This preserves device key custody and works with a mailbox server that never has the private keys, but retains crypto processing and a small device key index on the Mac. Cross-device key provisioning is explicit, not automatic.
- Helper-owned crypto: the operator explicitly provisions keys on the helper through a local administrative import. The helper exposes scoped crypto operations but never private-key export. This offloads crypto and permits unattended helper operations, but helper compromise then permits decryption/signing. A separate informed custody decision is required before implementing migration or remote secret import.

Recommended first implementation: device-local crypto. Existing code already prepares MIME before authenticated helper submission; remote startup intentionally omits the stores this path needs. Restore only device-owned crypto stores, not the mailbox database, sync workers, provider tokens or the local send transport. Do not reuse device TLS trust storage as mail identity storage.

## Authorization and account binding

Public-key discovery must remain explicit, bounded and subject to existing network protections. A helper token with mail.read or mail.send must not grant key administration, key export, or arbitrary signing of bytes. Device-local key operations require account/identity binding from helper account metadata, not a caller-supplied email alone.

If helper-owned crypto is later selected, use separate read-metadata, crypto-use and administrative grants, each bound to accounts and permitted identities. Validate recipients, key identity, validity and policy before signing/encrypting. Do not permit caller-selected file paths, commands, arbitrary URLs or algorithm choices. Revoking a device stops subsequent helper requests; it cannot revoke plaintext or keys already held by that device.

## Message preparation and retries

Apply the same signing/encryption policy as local mode. Missing keys, invalid certificates, required-recipient keys or locked key storage must fail closed, never downgrade to plaintext. Preserve To/Cc/Bcc envelope semantics while excluding Bcc headers from transmitted MIME. Tests must cover the existing serializer's exact behavior rather than assuming it.

Prepare immutable MIME once per stable send request. Retrying must reuse the same bytes and request ID, and inspect durable helper receipts before preparing or submitting after restart. Unknown provider acceptance must never become an automatic resend. Cancellation is valid only before provider submission; do not describe it as recall.

Sensitive plaintext, key material and passphrases must not enter logs, errors, analytics, HTTP URLs or action receipts. Bound MIME/attachment size, processing concurrency and decrypted-content lifetime. Do not persist decrypted bodies as an unbounded device cache. Security logs may contain operation IDs and content-free outcomes only.

## Implementation slices

1. Device crypto schema and explicit import: device-only startup, migration by deliberate user choice, no provider/mailbox DB dependency, preserve local mode behavior.
2. Account-bound key metadata, policies and public recipient discovery; restore remote compose capability checks only after their actual operations exist.
3. Remote read verification/decryption and immutable outgoing prepared-MIME path, including drafts, attachments and signing/encryption combinations.
4. Lifecycle and failure handling: key lock/revocation, offline helper, partial draft persistence, restart, ambiguous send acceptance and cleanup.

Each code PR needs failing tests before the fix. Minimum coverage: cross-account key misuse; no key transfer on connect; locked/missing/revoked keys; no plaintext fallback; Bcc confidentiality; expired/mismatched recipient certificates; payload/recipient/concurrency bounds; identical retry bytes; unknown-outcome no-resend; remote startup without a mailbox DB; local-mode regression tests. Record tests actually run and tests not run.

## Acceptance and unresolved decision

Review whether device-local crypto is the desired first parity path. Helper-owned custody is not inferred from a request for a lean frontend. No API or runtime behavior changes in this PR. Native OS-keychain prompts, real S/MIME/PGP interoperability, and two-machine delivery require validation outside this source-only environment before parity can be claimed.
