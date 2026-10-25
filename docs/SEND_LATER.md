# Send Later with a Mac Studio helper

P-011 now has a review-stage helper/SMTP path and composer controls. It is not marked complete or released: live delivery, keychain/login lifecycle, failure recovery and security regression verification are still outstanding. The initial transport is a direct helper connection; there is no hosted relay or paid hosted sender in this implementation.

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

The client would use the corresponding reachable HTTPS hostname. Redirects and untrusted TLS certificates are rejected; the pinned helper key is an additional E2EE identity check. Request size, concurrent request count and network timeouts are bounded. A public relay still needs additional quotas, retention policy, rotation and deployment work.

Once local setup works, install the macOS login agent using the same flags:

```sh
"$POSTILAATIKKO_HELPER" install-agent --identity studio
```

It starts at login and restarts after failure. Use `uninstall-agent` to stop and remove it before changing its configuration. A login agent requires that user to be logged in; the keychain must be available. It does not make a sleeping/offline Studio able to send. The mail UI can be closed while the helper runs. Restarting/upgrading during a send can leave an ambiguous outcome that needs inspection.

Custom app-data and queue locations are supported through explicit `--data` and `--queue` flags; use the same values for setup, serve, pairing and administration. The queue directory must be private (owner-only). Do not point it at a shared/home directory and do not upload its database to a relay.

## Schedule and manage messages

Pick a future local date/time in **Send Later**; the dialog shows the resolved timezone. The app saves the draft, prepares/signs/encrypts MIME once, seals the complete command for Studio, and durably saves those encrypted bytes locally before submission.

Only a verified encrypted acknowledgement from Studio's durable queue counts as accepted. The original draft is retained for recovery even after queue acceptance; this initial milestone does not delete it merely because a helper has queued it. Editing that draft does not change the helper's immutable message. Confirm cancellation before sending the draft again; after confirmed delivery, remove the original draft when ready. A dedicated Scheduled view and delivery-confirmed cleanup are future work.

If acknowledgement is lost, the app keeps the durable job ID. Use **Retry saved request**, which retransmits identical bytes, or **Refresh**; do not create a new job just because a network request timed out. Draft-linked uniqueness prevents scheduling the same draft twice. The Send Now path also checks for an existing scheduled job; simultaneous actions across separate windows still need concurrency regression verification.

The dialog lists the latest 100 jobs for the selected account with their last confirmed state. Open it from a new composer to inspect earlier jobs; a dedicated Scheduled folder is a later UI improvement. **Cancel delivery** succeeds only when Studio confirms it before delivery starts. Offline cancellation stays pending and must not be treated as recalled mail.

Raw MIME is limited to 10 MiB in this first implementation. Payloads are immutable after acceptance. Editing/rescheduling an existing job in the UI, streaming large attachments, remote relay transport and multi-device job discovery are not implemented yet.

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

Static paired keys do not provide forward secrecy for previously captured ciphertext. This does not itself encrypt ordinary outbound email for its final recipient; existing S/MIME/PGP settings are applied before scheduling. Live/helper login verification, tampering/replay/cancellation/duplicate-send regression checks, key rotation and security review remain required before release. See the [implementation plan](plans/2026-10-24-send-later.md) for the complete checklist.
