# Helper pairing handshake: protocol design for review

Status: design for review (HELPER-PAIRING-PROTOCOL-001). No code yet. The
handshake must pass external cryptographic review before release
(HELPER-PAIRING-001). The code format is a separate task
(HELPER-PAIRING-CODE-001); this design uses only fields that format already
carries.

## Goal

Pair a desktop with a helper on another computer without WebPKI
certificates, SSH, or copying a bearer token by hand. The only out-of-band
step is the user reading one short-lived code shown once by the helper.

## Threats in scope

- Network attacker between desktop and helper (active and passive).
- Anyone who reaches the helper's port before the owner does (first-contact
  takeover).
- Code shoulder-surfed or leaked after use, or guessed online.
- A relay that forwards a valid handshake from one TLS connection to another.

Out of scope: a compromised helper host or desktop (they are trusted
principals), and denial of service by an attacker who can reach the port
(rate limits only bound it).

## Inputs from the pairing code

Decoded by `internal/pairing` (HELPER-PAIRING-CODE-001):

- `address`: host:port of the helper's mailbox API.
- `certSHA256`: SHA-256 of the helper's TLS leaf certificate (DER).
- `secret`: 16 random bytes from a CSPRNG (128 bits).
- `expiry`: absolute time, at most 15 minutes after creation.
- `version`: 1. The helper identity fingerprint the format also carries is
  checked against the helper's Send Later public key after pairing; it is
  not needed for the handshake itself.

## Helper side: issuing a code

`postilaatikko-helper pair-code --new` (local CLI on the helper host only;
never a remote call):

1. Generate `secret`.
2. Store one row in the helper database: `lookup = SHA-256("postilaatikko-pairing-lookup-v1" || secret)[:16]`,
   `secret`, `expiry`, `failures = 0`, `used = false`. The database is 0600;
   the secret lives at most until `expiry` and is deleted on use, expiry or
   too many failures.
3. Print the code once (text and QR). Never log it.

At most one live code. Issuing a new one deletes any previous code.
Every read of the code table first deletes expired rows, and the running
helper also sweeps them at least once a minute, so the raw secret never
outlives `expiry` on disk by more than that interval.

## Transport

The desktop opens TLS 1.3 (HELPER-TLS13-001) to `address` and does not use
WebPKI. In `VerifyPeerCertificate` it computes SHA-256 of the leaf DER and
compares it to `certSHA256` in constant time; anything else aborts before
any request is sent. Server name and chain are not trusted for identity.

Both sides then derive a channel binding from the TLS session:

    ekm = ExportKeyingMaterial("EXPORTER-postilaatikko-pairing-v1",
                               context = version(1 byte) || lookup(16 bytes), 32)

(RFC 5705/8446 exporter; Go `tls.ConnectionState.ExportKeyingMaterial`.)
The helper reads it from `r.TLS` of the same request.

## Claim request

`POST /v1/pairing/claim` on the mailbox API listener. The route is mounted
only while a live, unused code exists; otherwise it returns 404 exactly like
an unknown route. It needs no bearer token.

Request body:

    {
      "version": 1,
      "lookup": hex(lookup),
      "deviceName": "<= 64 chars, display only",
      "deviceNonce": hex(16 random bytes),
      "proof": hex(HMAC-SHA256(key = secret,
                 "claim-v1" || ekm || lookup || deviceNonce || lp(deviceName)))
    }

Transcript encoding for every MAC: binary values are their raw bytes
(`lookup` is the 16 raw bytes, not its hex form; `ekm`, `deviceNonce` and
hashes likewise), and every variable-length field is length-prefixed as
`lp(x) = uint16 big-endian length || x`. Labels are fixed ASCII strings.

Helper checks, in order, and answers every failure with the same 403 and
the same body shape (no oracle about which check failed):

1. A code with `lookup` exists, is unused and not expired.
2. `failures < 5`. Otherwise delete the code.
3. The connection is TLS 1.3 and `ekm` is available.
4. `proof` verifies in constant time. On mismatch increment `failures`.

A request with an unknown `lookup` touches no code row, so an attacker who
does not know the code cannot burn its failure counter or lock it out;
only the global rate limit applies to such requests.

On success, in one database transaction: consume the code with a
conditional delete (`DELETE FROM pairing_codes WHERE lookup = ? AND
expiry > now RETURNING ...`), and only if that delete removed the row,
create an API grant (`RegisterClient`) with the default owner-device
capabilities (below) and record the device name and pairing time with the
grant. Two concurrent valid claims therefore yield exactly one grant.

The helper records the pairing in its audit log and prints it to the
running `pair-code` command if that is still open ("Paired: <device
name> at <time>"), and `api-client list` shows the device name and
pairing time, so the owner notices a pairing they did not make.

## Claim response and mutual confirmation

    {
      "clientId": "...",
      "token": "<bearer token, returned once>",
      "capabilities": [...],
      "sendLaterPublicKey": "<helper Send Later X25519 public key>",
      "confirm": hex(HMAC-SHA256(key = secret,
                   "confirm-v1" || ekm || lookup || deviceNonce || lp(clientId) || sha256(token)))
    }

The desktop verifies, in this order, before storing anything: `confirm`
(the peer knew `secret` on this exact TLS connection, in addition to the
certificate pin), then `sendLaterPublicKey` against the identity
fingerprint from the code. If either check fails, it discards the token
and makes a best-effort self-revoke call with it (a route that lets a grant revoke only itself, added with this work),
and tells the user to revoke `clientId` on the helper if that call fails.
Only after both checks pass does it store the token in the OS keyring and
the pinned certificate fingerprint and endpoint in device settings.

## Why not a PAKE

The secret is 128 random bits delivered out of band, not a human password,
so offline guessing after observing a transcript is infeasible and online
guessing is capped at five tries per code (and unknown-lookup requests
cannot exhaust those tries). HMAC with TLS channel binding
gives mutual proof of the secret bound to the connection. A PAKE (CPace,
OPAQUE) would be needed only if the code were shortened to something a
person could guess; keep the full 128-bit secret and this stays true.
Reviewer question: confirm this reasoning.

## Default capabilities

Owner's own device: `mail.read`, `mail.body.fetch`, `mail.write`,
`mail.send`, `account.manage`, plus contacts/calendar read capabilities
the desktop needs. Never key export or administrative crypto grants
(see `docs/plans/2026-11-13-remote-crypto-parity.md`).

## Later devices and rotation

- Additional desktops pair with a new code. Pairing from an already-paired
  desktop is a later option, not v1.
- TLS certificate rotation: out of v1. A changed certificate fails the pin;
  the user re-pairs with a new code. A signed-rotation scheme needs a
  helper signing key that does not exist today.
- Revocation stays `api-client revoke`; a Settings button for it is part of
  HELPER-PAIRING-001.

## Bounds

- Request body at most 4 KiB; device name at most 64 characters.
- Global rate limit on `/v1/pairing/claim`, for example 10 requests per
  minute, in addition to the per-code failure cap.
- The token, secret, proof and confirm values never appear in logs,
  errors, URLs or action receipts.

## Residual risk: a leaked code

A code read by someone else, plus network reach to the helper, gives that
person an owner-device grant (`mail.send`, `account.manage`) until the
owner notices. Mitigations in v1: the code lives at most 15 minutes, works
once, is shown only on the helper host, and every pairing is announced on
the helper (above). A confirm step on the helper host before the grant is
issued would close this; it is a reviewer question whether v1 needs it.

## Tests required before merge of the implementation

- Correct code pairs and returns a working grant; the code is then gone.
- Two concurrent valid claims: exactly one grant is created.
- Unknown lookup does not change the failure counter of the live code.
- Expired codes are removed from the database by the sweep.
- Identity-fingerprint mismatch after a valid `confirm`: the desktop
  stores nothing and revokes the returned grant.
- Wrong secret, expired code, used code, unknown lookup, sixth attempt:
  identical 403, and the code is deleted after five failures.
- Certificate mismatch aborts before the request is sent.
- Proof computed with another connection's `ekm` (relay) is rejected.
- Route is 404 when no live code exists.
- Desktop rejects a response whose `confirm` does not verify and stores
  nothing.
- TLS 1.2 connection is rejected.
- No secret or token in captured logs.

## Reviewer checklist

- Exporter label and context (now version and lookup).
- Whether v1 needs a confirm step on the helper host (residual risk above).
- Constant-time comparisons and the uniform failure response.
- Storage of the raw secret in the helper database for its lifetime.
- Whether five failures per code and the global rate limit are sufficient.
