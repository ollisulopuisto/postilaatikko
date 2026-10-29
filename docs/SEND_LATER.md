# Send Later with a Mac Studio helper

P-011 has a review-stage helper/SMTP path, composer controls, and a paged Scheduled view. It is not marked complete or released: live delivery, keychain/login lifecycle, native failure recovery and independent security review are still outstanding. Automated queue/recovery regressions now cover the boundaries listed below. The initial transport is a direct helper connection; there is no hosted relay or paid hosted sender in this implementation.

## Set up the Studio

Install Postilaatikko on the Studio and configure its mail accounts and sender identities there. Keep the machine awake and connected. The helper uses those local account credentials; clients never upload passwords or OAuth tokens.

macOS app builds bundle the helper at:

```sh
POSTILAATIKKO_HELPER="/Applications/Postilaatikko.app/Contents/MacOS/postilaatikko-helper"
"$POSTILAATIKKO_HELPER" init --identity studio
"$POSTILAATIKKO_HELPER" accounts
```

`init` prints the Studio's public key and creates its private key in the OS keyring. Run it once. Later, use `public-key --identity studio` to display the same public key. A missing/locked key stops delivery; the helper does not silently generate a replacement.

`accounts` prints the helper-local account IDs. Account IDs from another computer may differ, even for the same email address.

## Pair a composing device

1. In the composing device's Postilaatikko composer, choose **Send Later → Studio connection → Create device key**.
2. Read its public key directly on that device and grant it only the required account/sender pairs on Studio:

```sh
"$POSTILAATIKKO_HELPER" pair --identity studio \
  --client-key 'VERIFIED_DEVICE_PUBLIC_KEY' \
  --allow 'STUDIO_ACCOUNT_ID=sender@example.com'
```

Repeat `--allow` for more account/sender pairs in the same command. A new `pair` command for the same key replaces its permission set. To revoke a device, run `revoke --identity studio --client-key 'DEVICE_PUBLIC_KEY'`; pending jobs from that key are blocked. A delivery already inside SMTP cannot be recalled.

3. In the composing device's connection form, enter the Studio address, verify the Studio public key directly against its `public-key` output, and enter the helper-local account ID. Save the connection. Each local mail account has its own helper account mapping.

Keep the key comparison independent of an untrusted server or relay. Accepting a replacement key solely because a server supplied it would defeat E2EE.

## Run the helper

For the same Studio's app, start with the default loopback connection:

```sh
"$POSTILAATIKKO_HELPER" serve --identity studio
```

The app's default address is `http://127.0.0.1:8028`. Content and acknowledgements are still end-to-end encrypted and authenticated. Off-loopback connections require HTTPS with a certificate trusted by the client:

```sh
"$POSTILAATIKKO_HELPER" serve --identity studio \
  --listen '0.0.0.0:8028' \
  --tls-cert '/absolute/path/studio-certificate.pem' \
  --tls-key '/absolute/path/studio-tls-key.pem'
```

The client would use the corresponding reachable HTTPS hostname. Redirects and untrusted TLS certificates are rejected; the pinned helper key is an additional E2EE identity check. Request size, concurrent request count and network timeouts are bounded. The helper caps its logical queue at 10,000 job records and 512 MiB of encrypted envelopes. Existing jobs can still be cancelled at capacity. Revision tombstones are retained to reject delayed replays; there is no automatic history pruning. SQLite WAL and free pages can use additional disk space. A public relay remains a separate deployment with its own quotas, retention policy and rotation requirements.

Once local setup works, install the macOS login agent using the same flags:

```sh
"$POSTILAATIKKO_HELPER" install-agent --identity studio
```

It starts at login and restarts after failure. Use `uninstall-agent` to stop and remove it before changing its configuration. A login agent requires that user to be logged in; the keychain must be available. It does not make a sleeping/offline Studio able to send. The mail UI can be closed while the helper runs. Restarting/upgrading during a send can leave an ambiguous outcome that needs inspection.

Custom app-data and queue locations are supported through explicit `--data` and `--queue` flags; use the same values for setup, serve, pairing and administration. The queue directory must be private (owner-only). Do not point it at a shared/home directory and do not upload its database to a relay.

## Schedule and manage messages

Pick a future local date/time in **Send Later**; the dialog shows the resolved timezone. The app saves the draft, prepares/signs/encrypts MIME once, seals the complete command for Studio, and durably saves those encrypted bytes locally before submission.

Only a verified encrypted acknowledgement from Studio's durable queue counts as accepted. Offline submissions retain their exact encrypted request for **Retry saved request**. Do not create another job because a network request timed out. Draft-linked uniqueness and atomic SQLite arbitration across composer processes prevent a draft from winning both Send Now and Send Later. A Send Now attempt leaves a small durable marker even on failure, since the provider might have accepted it: inspect delivery before using a new draft to schedule that content. Ordinary Send Now retry behavior is unchanged; this is not an exactly-once SMTP guarantee.

The **Scheduled** pane provides all-account or per-account filters, active/history pages of 40 messages, encrypted local previews, last confirmed status, cancellation, time-only rescheduling, and bounded status refresh. It is mounted by the main app when the `scheduled` view is selected. Its sidebar entry is left for coordinator integration with the concurrent sidebar changes. Composer dialogs retain their per-account latest-100 list, including detached composers. The pane never loads attachments during normal refresh; two visible statuses are refreshed per 15-second interval, and polling stops when the pane closes. Locked/unavailable client keys allow cached status metadata, while previews and actions require the key.

**Cancel delivery** succeeds only when Studio confirms it before delivery starts. Offline cancellation stays pending and must not be treated as recalled mail. **Reschedule** submits a higher authenticated revision with the new future time, preserving the prepared MIME, Message-ID, signatures and original Date header. Changing recipients, content or attachments requires cancelling the old job and composing a new one. If delivery already started, the rejected change reconciles to the helper's real status instead of claiming success. A missed due time is delivered when the awake, authorized helper next processes its queue; an ambiguous interrupted SMTP attempt instead requires inspection.

The original draft remains until this client refreshes a delivery-confirmed `sent` status. Cleanup compares the scheduled draft fingerprint and exact local row version. Synced drafts additionally require matching UIDVALIDITY, application draft markers, and a streamed body hash check before deleting the recorded remote UID. Edited drafts, legacy drafts without proof, or remote content that cannot be verified are preserved with a visible cleanup outcome. A save racing local deletion creates a new draft instead of losing the edit. Sent-copy failures preserve the original draft and encrypted payload for manual repair; queue acceptance alone never deletes a draft. Cleanup does not run while the composing device/app is offline, and does not affect routine header-only sync.

Raw MIME is limited to 10 MiB. Prepared payloads are immutable; large attachment streaming, relay transport, automatic Sent-copy repair and multi-device job discovery remain outside this helper milestone. Paired static keys have no forward secrecy, and key rotation/history retention still need an explicit migration design. Keep queue metadata/tombstones when backing up or moving a helper, so old encrypted submissions cannot be replayed as new deliveries.

## Ambiguous SMTP results

If a connection fails after message data was submitted, the provider may already have accepted it. Studio records `outcome_unknown` and does not automatically resend. Check the provider's Sent folder and delivery evidence before resolving it locally:

```sh
"$POSTILAATIKKO_HELPER" list --identity studio
"$POSTILAATIKKO_HELPER" resolve --identity studio \
  --job 'JOB_UUID' --revision 1 --outcome sent
```

Use `--outcome failed` only after confirming the provider did **not** accept the mail. It is not an instruction to retry blindly. A lost acknowledgement to SMTP does not become exactly-once delivery merely because the MIME has a stable Message-ID. Sent-copy errors after provider acceptance are recorded as sent with a warning and retained encrypted payload; they never trigger a resend. Automatic Sent-copy repair remains future work.

## Security scope and remaining review

E2EE protects composing-device ↔ trusted-helper content and status responses, including recipients, attachments, account choice and due time. Public keys, sizes and traffic timing remain visible to a relay. Helper-local status metadata is not sent in plaintext. Private keys use the OS keyring without plaintext fallback; mail credentials remain on Studio in its existing credential store.

Static paired keys do not provide forward secrecy for previously captured ciphertext. This does not itself encrypt ordinary outbound email for its final recipient; existing S/MIME/PGP settings are applied before scheduling. Live/helper login verification, provider fault checks, key rotation and independent security review remain required before release. Automated regression coverage is described below; it does not establish real delivery or a complete security audit. See the [implementation plan](plans/2026-10-24-send-later.md) for the complete checklist.


## Automated verification and native acceptance

P-011-SEC adds real encrypted-command and SQLite tests for lost durable acknowledgements with exact-byte retry, old receipt rejection on fresh queries, cross-client ownership, stale status after confirmed cancellation, cross-process Send Now/schedule arbitration, and concurrent helper claim/cancel/reschedule behavior. Recovery tests cover active-claim grace periods, stale worker completion, revision-bound manual resolution, transport failure classes and retry limits, and an injected database completion-write failure after provider acceptance. Ambiguous acceptance never automatically retries; recoverable encrypted payloads remain available.

A regression using a temporary app database and an in-process mock keyring reproduced missing Gmail OAuth credentials incorrectly becoming `outcome_unknown` before any provider submission. Local credential lookup/refresh failures now retain the authorization-required classification, consistent with SMTP. This test does not access the login keychain or a real account.

The Mac Studio acceptance gate remains open: verify direct and remote paired delivery with the mail UI closed; cancellation/rescheduling before and during provider submission; sleep/offline missed times; helper termination/restart and reboot/login-agent recovery; locked/missing keys and expired/revoked credentials; ambiguous provider acceptance; Sent-copy failure and draft cleanup racing live edits. Check real Gmail/SMTP/IMAP, recipient/Bcc/attachment fidelity and provider Sent evidence. Automated SQLite and HTTP fault injection cannot substitute for those checks. See the [P-011-SEC handoff](diagnostics/2026-10-29-send-later-security-recovery.md) for reproducible commands and limits.
