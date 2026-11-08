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

The detached `ComposerApp` does not open the shared database or initialize
account, credential, message, draft, folder, IMAP, OAuth, or crypto stores. Its
mailbox operations cross authenticated IPC to the main process. Composer IPC
has a bounded 30 MiB payload limit for attachments; the smaller reader-call
limit is unchanged.

The parent now routes detached compose account/identity/original-message reads,
draft CRUD, reply preparation, and immediate send through authenticated helper
APIs. A helper disconnect produces an error while the composer retains its
in-memory fields; these calls do not fall back to local database writes. Draft
read/write and mail-send permissions are separate explicit grants. Draft body
encryption and signing remain device-local; the helper stores opaque encrypted
payloads. New-draft create IDs and send idempotency keys remain stable through
same-session retries. Saved-draft send keys derive from the draft revision, and
the desktop checks the helper's durable receipt before preparing another send;
safe-retry receipts derive a stable next attempt key. The helper performs
provider token refresh and delivery, including Gmail API threading using the
compose message's Gmail thread ID.

Reply preparation explicitly fetches the original body through the helper and
returns inline and forward attachments from helper-owned storage/provider
fetches. Original-message header reads omit body content.

Focused `go build ./app ./cmd/postilaatikko-helper`, locale JSON parsing, and
`git diff --check` pass. Tests were not run. The frontend static checker is
unavailable because `frontend/node_modules` is absent. Live SMTP, Gmail, and
native composer behavior remain unverified.

## Review risks / limits to resolve

- Confirm live Gmail threading and SMTP delivery after install; focused builds
  do not verify provider acceptance or native composer behavior.
- Sender certificate and PGP material stays local, and the desktop passes the
  prepared signed/encrypted RFC822 payload to the helper. Device verification
  remains necessary for those crypto combinations.
- Unsaved messages have no helper draft revision to anchor a durable key. A
  process restart during an ambiguous send of an unsaved message still needs
  receipt recovery before a new send attempt is safe.
