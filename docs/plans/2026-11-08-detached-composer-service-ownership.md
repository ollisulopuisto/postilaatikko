# Detached Composer Service Ownership

## Goal

Remove the detached composer process's direct access to service-owned mailbox
database rows and provider credentials. Preserve device-local contact search,
certificate/PGP key UX, and local crypto operations behind an explicit boundary.

## Current evidence

`app/detached_composer.go:156-225` opens `aerion.db`, constructs account,
folder, message, draft, settings, and credential stores, initializes provider
connection pools and OAuth refresh, and wires them into `composeOps` and
`draftOps`. Draft load/save/delete and send directly use those stores. The
composer currently remains functional when its main-window IPC connection is
unavailable, so service delegation must report unavailable operations without
silently falling back to shared-database writes.

UX-037 provides authenticated helper routes and client calls for account
catalog, draft CRUD, cached-body reply preparation, and immediate send. Send
accepts prepared RFC822 bytes and account-authorized SMTP envelope recipients.
The composer can continue to read OS-local contacts and perform local signing
and encryption; private key material must not move to the helper through this
task.

## Ownership boundary

- Helper-owned: account/identity reads, mailbox-backed draft CRUD and revision,
  reply preparation from fetched message data, provider credentials and token
  refresh, send transport and its durable idempotency receipt.
- Device-local: macOS Contacts and shared vCard lookup; private key stores,
  trust decisions and cryptographic signing/encryption UX; transient compose
  state before a draft is acknowledged by the helper.
- The detached composer must not open `aerion.db`, create account/draft/folder/
  message/credential/settings stores, or obtain provider credentials.
- On helper disconnect, preserve the in-memory compose contents and surface a
  retryable save/send error. Never report success or write the old shared DB as
  fallback.

## Implementation sequence

1. Inventory the detached composer's Wails methods and their store/provider
   dependencies; classify each as helper-owned or device-local.
2. Add an authenticated helper client to the detached process through the
   existing API-token/configuration handoff. Verify account/identity reads and
   draft load/create/update/delete use service responses and revisions.
3. Keep contact, certificate, PGP and S/MIME stores device-local through an
   appropriate local store, without retaining a mailbox database handle.
4. Prepare RFC822 locally with the existing compose pipeline and delegate
   immediate delivery to `/v1/send`; retain the same idempotency key across
   retries and clear the draft only after the service receipt says sent.
5. Remove direct credential refresh/provider configuration from composer.
   Refresh is helper-owned; report reauthorization-required responses to UI.
6. Update lifecycle, docs and task ledger. Run focused Go build and
   `git diff --check`; do not claim live provider or UI validation.

## Progress checkpoint

The detached `ComposerApp` no longer opens the shared database, initializes a
credential store, or creates account/draft/message/folder/IMAP/crypto stores.
Its account, identity, message, draft, reply, send, contacts, key, draft-history,
pending-send, and scheduling calls now cross the authenticated IPC connection
to the main process. Composer IPC has a bounded 30 MiB payload limit for
attachments; the existing smaller reader-call limit is unchanged.

This is an ownership boundary for the child process, not yet the helper handoff:
the main process methods still use some local account/draft/send/token stores
and transports. Route those methods through UX-037 and the account APIs, add
identity reads, enforce explicit `draft.read`, `draft.write`, and `mail.send`
capabilities, then resolve encrypted draft persistence and Gmail provider-send
semantics before removing the parent fallback.

Focused `go build ./app ./cmd/postilaatikko-helper` and `git diff --check` pass.
Tests and live provider/UI checks were not run.

## Review risks / limits to resolve

- UX-037's current helper draft API rejects encrypted-body draft persistence.
  The composer must not silently save plaintext where the previous path stored
  encrypted draft content. Choose a safe explicit failure or add a protected
  desktop-local encrypted draft cache before claiming parity.
- UX-037 currently delivers through helper-local SMTP transport. Confirm Gmail
  account delivery semantics and provider-specific threading before removing
  the composer Gmail send path.
- Sender certificate and PGP material stays local; verify preparation produces
  the final signed/encrypted RFC822 payload before handing it to the helper.
- In-flight edits and send/draft failure must remain recoverable in the live
  composer session. Delivery ambiguity must reuse the existing idempotency key.
