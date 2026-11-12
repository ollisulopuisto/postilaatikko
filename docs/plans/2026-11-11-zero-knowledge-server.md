# Zero-knowledge Postilaatikko service: design (2026-11-11)

Owner requirements (2026-11-11, paraphrased): client to server end-to-end
encrypted and quantum-safe; a server compromise must not reveal the owner's
mail; offered as a hosted multi-tenant service; "so secure even admin can't
reach inside" - the operator, including a malicious or legally compelled
operator, must be unable to read mail content.

This document is a design only: no code changes. Claims are marked
**[documented]** (current repo code/docs, with file references) or **inferred**
(reasoning, platform/provider behavior, or standard practice; provider policies
cited here need re-verification before commitments are made to users). All
cryptography uses vetted primitives; nothing is invented here. Sections that
combine primitives into a protocol are flagged for expert review.

## 1. The claim we can defend - and the one we cannot

**"100% secure" is not achievable, and we must not say it.** Not for this
system, not for any system. Every additional claim below is bounded.

The strongest defensible claim, if this design is implemented and externally
audited:

> The service stores and transmits only ciphertext. Mail content, search
> queries, and IMAP/OAuth credentials never leave a user's device unencrypted.
> An operator with full administrative access to the servers - or an attacker
> who compromises them - cannot read any user's mail content.

Caveats that travel with that claim, always:

- **Metadata remains visible**: account existence, blob sizes and counts,
  access timing, IP addresses. Not content. (Section 14.)
- **Availability is not guaranteed** against a malicious operator: the operator
  can delete or withhold ciphertext. Zero knowledge protects confidentiality,
  not durability.
- **Endpoints are out of scope**: a compromised or stolen unlocked device can
  read that user's mail. (Threat model, T5.)
- **Scheduled sending is a carve-out** unless sending moves to the client: with
  helper-side delivery the server holds the scheduled message in plaintext at
  delivery time *and* the provider credentials needed to send it. For tenants
  who use it, the "credentials never leave a device" sentence above is false.
  (Section 10.)
- The claim holds only for **native clients whose builds the operator cannot
  alter unnoticed**. A web client served by the operator defeats it, and so
  does a native build signed by the operator unless independent parties can
  verify it against public source (T6; section 2, item 3).

## 2. What cannot be met, stated up front

1. **Server-side incoming sync is incompatible with zero knowledge.** Syncing
   IMAP/Gmail requires the user's credentials and processes plaintext. The
   current helper does exactly this **[documented: docs/SEND_LATER.md:71,88;
   docs/REMOTE_MAILBOX_SERVICE.md:43]**.
   In this design, sync happens on client devices only; the server never sees
   provider credentials - with one exception: helper-side Send Later delivery
   needs sending credentials on the server (section 10).
2. **A malicious operator cannot be stopped from attacking availability.**
   Deleting blobs, refusing service, or rolling back state are outside what
   cryptography prevents. Mitigations are backups the user can verify and
   export, and contractual/legal exposure for the operator. **[inferred]**
3. **A web client served by the operator cannot be trusted.** An operator (or
   whoever compromises the web deployment) can ship JavaScript that exfiltrates
   keys or plaintext. Zero knowledge to the operator requires signed, native
   client applications. Native alone is not enough here: the operator of the
   hosted service is also the publisher who signs the client, so a malicious
   or compelled operator can ship a build that exfiltrates keys. The claim
   against T2 therefore depends on public source plus reproducible builds that
   independent parties actually verify (and, ideally, a public log of released
   binaries). Until that exists, the honest claim is "the server cannot read
   mail", not "the operator cannot". Whether reproducible builds are a launch
   requirement is an owner decision (section 22). **[inferred; standard
   argument in the E2EE industry]**
4. **Post-quantum server authentication is not available from the public
   WebPKI today.** CAs issue classical certificates only. Transport key
   exchange can be hybrid post-quantum; server identity is anchored
   classically, mitigated by application-layer pinning of a service identity
   key (the Send Later path already pins a helper key **[documented:
   docs/SEND_LATER.md:63]**).
5. **Leakage-free server-side search does not exist in practice.** Section 11.
6. **Traffic-analysis resistance is out of scope.** Sizes, timing, counts leak
   unless padding/batching is added; this design does not attempt it. **[inferred]**

## 3. Threat model

| # | Adversary | Protected | Not protected |
| --- | --- | --- | --- |
| T1 | External attacker with full server compromise | Mail content, credentials, search queries (all ciphertext client-side) | Metadata; availability |
| T2 | Malicious or compelled operator (root on everything) | Same as T1, because keys never leave clients | Metadata; availability; scheduled mail and sending credentials under helper-side Send Later (section 10); can serve an evil web client (so: native clients only) and, as publisher, a malicious native update (T6) |
| T3 | Malicious tenant | Other tenants' content (per-user keys, tenant isolation, section 6) | Shared-infrastructure metadata (timing on shared queues) **[inferred]** |
| T4 | Network attacker (incl. on the tailnet/ISP path) | Content (app-layer E2EE) and recorded-traffic decryption (hybrid PQ TLS) | Traffic analysis |
| T5 | Stolen/lost endpoint | Locked device with FileVault/keychain protects at rest; revocation (section 12) limits a live theft | An unlocked, logged-in device exposes that user's mail |
| T6 | Malicious client update (incl. one signed by the operator as publisher) | Only when native builds are reproducible from public source and independently verified | Operator-served web clients (unfixable); native builds nobody can verify |

## 4. Current state (baseline, all documented)

Single-user helper today:

- Mailbox API: TLS `MinVersion: tls.VersionTLS12`, default curves
  (`internal/automation/server.go:178`); Go `http.Client` desktop, no pinning
  (`internal/automation/client/client.go:56`); per-client bearer tokens, only
  SHA-256 digests stored (`internal/automation/sqlite_token_store.go:66`).
- Helper is fully trusted with credentials and mail; "not end-to-end encrypted
  from the service operator" (`docs/REMOTE_MAILBOX_SERVICE.md`).
- Send Later: E2EE composer-to-helper, NaCl box, static X25519, no forward
  secrecy (`internal/scheduledsend/protocol.go`, `identity.go`;
  `docs/SEND_LATER.md:116`); queue capped 10,000 jobs / 512 MiB encrypted
  envelopes (`docs/SEND_LATER.md:63`).
- Mail at rest: unencrypted SQLite (`internal/database/database.go:75`);
  credentials in OS keyring (`internal/keyring/keyring.go`).
- The desktop already keeps a local mailbox DB with FTS
  (`internal/database/migrations.go:413`; `docs/REMOTE_MAILBOX_SERVICE.md`):
  client-side search infrastructure already exists.

## 5. Target architecture (multi-tenant)

Design principles (owner, 2026-11-11):

1. **The server must not hold the mailbox.** Mail lives on client devices.
   Client-side IMAP/Gmail sync is the only sync; there is no server-side copy
   of the mailbox, encrypted or otherwise, by default.
2. **The server is limited to what is strictly needed**: the Send Later helper
   (already E2EE), and optionally small, selective encrypted transfer between a
   user's own devices - never a full server-held archive.
3. **The current mailbox-service mode moves to opt-in, then deprecated**
   (section 20): it is the mode where the server holds mail, which this design
   retires.

**Send Later helper (existing, upgraded cryptography, section 10).** The one
standing server role. Today it is a helper that terminates the encryption and
delivers the mail itself, not a blind relay; there is no hosted relay yet
**[documented: docs/SEND_LATER.md:3]**. At rest it holds encrypted
scheduled-outgoing envelopes; with helper-side delivery it also holds the
sending credentials and sees each message in plaintext when it is sent.

**Optional device-transfer envelopes (new, opt-in, selective).** A user who
wants another device to catch up without a full IMAP pull may encrypt
*selected* recent mail (bounded window/folders, user-chosen) into envelopes the
server stores briefly; the server never accumulates a whole mailbox. A server
of opaque envelopes with a TTL, not an archive. Direct device-to-device
transfer (e.g. over the tailnet) is the preferred alternative where devices are
ever online together. **[inferred]**

**Sync (clients only).** Each client syncs IMAP/Gmail itself - the desktop
already does **[documented]** - and keeps its own local mailbox and cache,
protected by full-disk encryption.

**Clients: native only** (macOS desktop exists; mobile later). No operator-served
web client (T6 / section 2, item 3).

**Sending:** client-side via the user's own provider credentials (see section
10 for Send Later).

**Service identity:** long-lived Ed25519 service key, pinned by clients,
anchoring app-layer identity independent of the WebPKI (section 2, item 4).

**Storage sizing per tenant** (owner asked 2026-11-11):

| Mode | Server holds per tenant | Order of magnitude |
| --- | --- | --- |
| Mailbox service (current helper, deprecated path) | full mailbox copy + FTS index + SQLite WAL/free-page overhead **[documented WAL note: docs/SEND_LATER.md:63]** | several GB per active tenant, growing forever [inferred; matches the owner's estimate] |
| Send Later-only | encrypted outgoing envelopes + tombstones, capped **[documented: 10,000 jobs / 512 MiB, docs/SEND_LATER.md:63]**; with helper-side delivery also the tenant's sending credentials | tens of MB typical; 512 MiB ceiling |
| Client-held mail (this design) | tokens, public keys, wrapped-key blobs, tombstones | KBs to low MBs |
| + optional transfer envelopes | user-chosen recent window, TTL'd | tens-hundreds of MB, self-pruning [inferred] |

Client-held mail keeps hosted storage near zero per tenant: minimal cost, and
minimal breach blast radius. [inferred]

## 6. Tenant isolation

- Per-user namespaces; every object keyed by unguessable IDs (random 128-bit+).
- Capability tokens scoped per user and per operation class; no cross-tenant
  list/read primitives; per-tenant rate limits (noisy-neighbor + abuse).
- Cryptographic isolation is per-user keys (section 7), not per-tenant server
  keys: the server cannot decrypt any tenant, so a server-side isolation bug
  degrades to metadata confusion, not content exposure. This ordering (crypto
  first, ACLs second) is the design's core safety property. **[inferred]**
- AAD binds every envelope to its user's namespace so ciphertext cannot be
  replayed across tenants. **[inferred]**
- Shared queues/caches: none holding per-tenant data beyond unavoidable
  infrastructure telemetry (T3).

## 7. Key hierarchy and where keys live (per user)

Scope: the server holds no mailbox (section 5), so these keys do not encrypt a
server-side archive. They protect the only user data that reaches the server:
optional device-transfer envelopes and the wrapped key blobs used for
enrolment and recovery. The local mailbox on each device is protected by the
device's own disk encryption, not by this hierarchy.

- **User Master Key (UMK)**: 256-bit, generated on the user's first enrolled
  client (CSPRNG). Never leaves clients unwrapped.
- **Per-envelope content keys (CEK)**: random 256-bit; XChaCha20-Poly1305 for
  envelope ciphertext (192-bit nonces, safe random); CEK wrapped to UMK;
  HKDF-SHA-256 context separation (RFC 5869).
- **Device keys**: per device, a signing keypair and a KEM keypair. Apple
  platforms: signing key as Secure Enclave P-256 **[inferred platform
  constraint: Secure Enclave supports secp256r1 only]**; KEM private keys in
  the OS keychain. Target: ML-KEM-768 (FIPS 203) per-device KEM keys; hybrid
  X25519+ML-KEM-768 wrap from day one so stored wrapped keys are
  PQ-confidential.
- All private key material lives on user devices; the server stores only
  ciphertext, public keys, wrapped blobs. **[inferred design; vetted primitives
  only]**

## 8. Envelope format (v1)

Self-describing device-transfer envelope: header (version, namespace, random
envelope ID, CEK wrap, creation and expiry timestamps) + ciphertext; AAD binds
version, namespace, envelope ID and expiry. Provider identifiers (account,
folder, UIDVALIDITY, UID, Gmail IDs) travel inside the ciphertext, not in the
header, so the server learns nothing about mailbox structure. **Expert review
flag** before implementation. **[inferred]**

## 9. Transport security

- TLS 1.3 only, on both listeners. The mailbox API is a one-line change
  (`tls.VersionTLS13` at `internal/automation/server.go:178`). The Send Later
  listener - the one that stays - sets no TLS config of its own and so accepts
  Go's default minimum, TLS 1.2 (`server.ServeTLS` at
  `cmd/postilaatikko-helper/main.go:456`); it needs an explicit
  `TLSConfig` with the same minimum. **[documented locations; inferred
  effect]**
- Hybrid post-quantum key exchange X25519MLKEM768 is Go's TLS 1.3 default since
  Go 1.24 **[inferred: Go release notes]**; Go clients and server get it free.
  Non-Go clients (future mobile) need per-platform verification.
- App-layer E2EE makes TLS defense in depth, not the boundary.

## 10. Sending and Send Later under the headline requirement

- **Immediate sending**: client-side, user's own SMTP credentials, never
  touching the service. No relay, no abuse surface (section 17). **[inferred]**
- **Send Later**: someone must hold plaintext MIME at delivery. Options:
  (a) helper-side delivery (current design, upgraded crypto below) - a live
  operator/compromise at delivery time can read that payload, and the helper
  must hold the tenant's sending credentials the whole time: today it sends
  with account credentials stored on the helper host **[documented:
  docs/SEND_LATER.md:7]**. In a hosted service that puts every such tenant's
  provider credentials in the operator's hands. Where the credential also
  grants mailbox read access (an IMAP/SMTP password or app password, or a
  full-mail OAuth scope), the operator can read that tenant's whole mailbox,
  not one scheduled message. This violates the headline for those tenants. It
  can be narrowed, not removed, by accepting only send-only credentials where
  the provider offers them (for example a send-only Gmail API scope instead of
  SMTP) **[inferred; verify per provider]**; (b) client-side delivery when
  any enrolled client is online, else the job waits - meets the headline,
  degrades scheduling reliability; (c) drop Send Later from the hosted
  service. **Owner decision (section 22).**
- If helper-side delivery is kept, upgrade its crypto: ephemeral per-submission
  hybrid X25519+ML-KEM-768 KEM (X-Wing construction or HPKE hybrid DHKEM;
  **expert review flag**), rotating helper KEM keys (rotation already needed
  **[documented: docs/SEND_LATER.md:116]**), per-job key deletion on
  completion for forward secrecy, helper receipts signed with the service
  identity key.

## 11. Search over encrypted data

| Option | Server learns | Effort | Notes |
| --- | --- | --- | --- |
| S1. Client-side search only | nothing | None new | Mail is already in the client-local DB from client-side sync; search uses the existing FTS **[documented: messages_fts, internal/database/migrations.go:413]**. The server is not involved. Recommended. |
| S2. Deterministic keyword tokens (e.g. AES-SIV) | keyword frequency/equality; repeated queries | Medium | Frequency-analysis attacks known. Not recommended. |
| S3. Structured searchable encryption (OXT-style) | formal leakage profile | Large | **Expert review mandatory.** |

S2 and S3 only make sense over a server-held mailbox, which principle 1
rejects. They are listed to record why they were not chosen, not as planned
work.

**Server-held indexes (owner question, 2026-11-11: "indexes maybe yes?").**
An index is only useful to the server if the server also holds the ciphertext
it points into - which principle 1 rejects for the mailbox. A blind/encrypted
index over a server-held mailbox would leak query and frequency patterns
(S2/S3 above) *and* force the server to hold all mail. For the small, TTL'd
transfer-envelope set, the client can simply download and index locally. So:
**default and recommendation is client-side cache + local index** (the desktop
already maintains one **[documented: messages_fts,
internal/database/migrations.go:413]**); server-held indexes belong to the
deprecated mailbox-service mode and retire with it. [inferred]

## 12. Multi-device enrolment and revocation

- Enrol: new device generates device keys; an existing device verifies a short
  authentication string out-of-band, wraps the UMK to the new device's hybrid
  KEM public key, uploads the wrapped blob. **Expert review flag** on the
  ceremony (compare published Signal/1Password models). **[inferred]**
- Device list: the set of enrolled devices (each device's signing and KEM
  public keys, plus the recovery public keys from section 13) is a
  versioned, hash-chained list: every version names the hash of its
  predecessor and is signed by a device, or the recovery signing key, listed
  in that predecessor. The server stores and relays it but cannot add,
  replace or roll back a key. A client never wraps the UMK, or anything
  else, to a public key that is not in its own verified list; a key the
  server supplies outside that list is ignored and reported. **[inferred]**
- Trust anchors, so every device starts from an authenticated list:
  - First device: creates version 1 with itself and the recovery public
    keys.
  - New device: receives the current list from the enrolling device inside
    the SAS-verified enrolment channel, never from the server alone.
  - Recovery after losing every device: the replacement derives the
    recovery keys from the phrase, fetches the chain from the server and
    accepts its latest version only if that version lists those exact
    recovery public keys and every link verifies. The recovery signing key
    then signs the next version, adding the replacement device. The phrase
    is the anchor; the server's copy is only transport.
- Forks: a revoked or stolen device stays a valid signer in any predecessor
  that lists it, so it can sign a competing successor, and a colluding
  server can show that fork to a device that missed the real revocation.
  Countermeasures:
  - Before a device wraps the UMK to anything (enrolment, rotation), it
    fetches the latest chain and shows the user every device added since its
    own last verified version, requiring confirmation. A silent update never
    leads to a wrap.
  - Devices sign and exchange the chain head they have seen whenever they
    talk (enrolment channel, transfer envelopes), so two successors of the
    same predecessor are detected; a detected fork freezes all wraps until
    the user resolves it on a trusted device.
  - A longer-term option is an external, append-only transparency log of
    list heads.
  This is the key-transparency problem; implement a published design
  (compare WhatsApp key transparency, Apple Contact Key Verification,
  Keybase sigchains), not a home-grown one. **Expert review flag.**
  **[inferred]**
- Revoke: delete the device's wrapped-UMK blob, revoke its token (documented
  `api-client revoke` machinery as the pattern), and rotate the UMK so new
  transfer envelopes are unreadable to the revoked device. Pending envelopes
  are not deleted: a pending envelope may hold the only copy of its mail
  (the provider no longer has it and the source device may be gone). Instead,
  the device performing the revocation re-wraps each pending envelope's CEK
  from the old UMK to the new one and uploads the new wraps; the envelope
  ciphertext stays unchanged. This needs only the old UMK, not the device
  that created the envelope, and cuts the revoked device off once its token
  is refused. It does not protect envelopes the revoked device already
  downloaded, or ones a colluding operator hands it with their old wraps;
  where that matters, the revoking device re-encrypts the pending envelope
  under a fresh CEK instead (re-upload, same TTL). An envelope is deleted on
  revocation only when the user chooses to.
- Rotation order, so no step can strand a pending envelope (an interrupted
  rotation resumes from the first unfinished step; every step is idempotent):
  1. Sign and upload the new device-list version without the revoked
     device. Generate the new UMK and, before anything else, upload it
     wrapped to every surviving device's KEM public key and to the recovery
     key (section 13), taking those keys only from the revoking device's own
     verified device list, never from the server's answer. Offline devices
     need nothing from the revoking device: they verify the new list and
     fetch their new wrap when they next connect.
  2. Only after the server confirms step 1, upload the new CEK wraps for
     pending envelopes *alongside* the old ones.
  3. Only after the server confirms step 2, delete the old wraps and the
     revoked device's wrapped-UMK blob.
  If the revoking device is lost mid-rotation, any surviving device or the
  recovery key still holds whichever UMK each envelope is currently wrapped
  under, and finishes the rotation. **Expert review flag** on this protocol.
  **[inferred]**
- Revocation does not touch the mail the device
  already synced or its provider credentials; the user must also revoke that
  device's access at the provider (password change / OAuth grant removal). A
  live stolen unlocked device exposed what it read (T5).
- Lost last device: mail that still exists at the provider is not lost - a
  new device syncs it again. Sync cannot restore what the provider no longer
  holds: provider-deleted mail and local-only drafts that never left the old
  device are gone with it, with one narrow exception. Recent mail a device
  selected into a transfer envelope sits at the server until that envelope's
  TTL expires, and the recovery phrase does permit its retrieval before
  then: the phrase restores the UMK and each envelope's content key is
  wrapped to the UMK (section 7), so a recovered UMK decrypts any envelope
  still pending. Once an envelope's TTL lapses it is pruned and gone, phrase
  or not. What is lost without the recovery phrase is the UMK and anything
  still waiting in transfer envelopes. No administrative backdoor, by
  design. Tell users this in plain words.

## 13. Backup and recovery

- Recovery phrase: 256-bit seed as a standard word-list phrase or base32,
  written down / password-manager escrow; optional second escrow (e.g. iCloud
  Keychain). **Owner decision.**
- The phrase is not the UMK. It deterministically derives a recovery KEM
  keypair (hybrid, like device keys) and a recovery signing keypair. Both
  public keys are on the signed device list (section 12); the server stores
  only those public keys, and the current UMK is always wrapped to the
  recovery KEM key like to another device. The recovery signing key lets a
  user who lost every device sign the next device-list version. UMK
  rotation (section 12) re-wraps to the recovery public key without the
  phrase being present, so the phrase the user wrote down stays valid across
  rotations. **[inferred]**
- Server backups are ciphertext: safe by construction. Keep Send Later
  tombstones **[documented: docs/SEND_LATER.md:116]**.
- Exit is not hostage to the operator: the mailbox lives at the provider and
  on the user's devices, so an operator who deletes or withholds data can
  cost the user pending scheduled sends and transfer envelopes, never the
  mailbox (mitigates T2 availability). **[inferred]**

## 14. Metadata exposure and minimisation

Visible to the operator even in this design: account existence and identifiers
(signup identity, payment), IP addresses and connection timing, blob counts/
sizes/timing, per-tenant request rates. **[inferred]**

Minimisation commitments to design for:

- No content in logs anywhere (section 18).
- Retain connection/IP logs for the shortest operationally safe window;
  publish the retention table.
- Aggregate billing/usage telemetry; no per-message records server-side (the
  server cannot see message boundaries inside envelopes, only envelope sizes -
  keep envelopes padded to size classes if we want to reduce size leakage;
  padding is a later, optional hardening). **[inferred]**
- Publish a data-map: exactly what the service stores per user, so the claim in
  section 1 stays auditable.

## 15. Gmail / Microsoft OAuth constraints for hosted third-party access

- **Client-side sync keeps provider tokens on devices.** The service never
  presents OAuth tokens to Google/Microsoft, which avoids the deepest
  third-party-access obligations. **[inferred]** This holds only without
  helper-side Send Later: a hosted helper that sends on the user's behalf
  holds and presents provider credentials, and brings the service itself into
  scope of the providers' third-party-access reviews (section 10).
- The app itself still uses Google restricted scopes for Gmail access, so it
  needs Google's OAuth verification, and - while restricted scopes are in use -
  an annual security assessment under Google's CASA framework (tier depends on
  data handling; verify the current tier requirements before launch).
  Microsoft's ecosystem requires app registration and publisher verification,
  with its own review for broad mail scopes. **[inferred: provider policies as
  of writing; re-verify - they change]**
- Consequence: a hosted-sync variant (section 2, item 1) would also pull the
  service itself into the heaviest CASA tier and make the zero-knowledge claim
  false. Another reason sync stays client-side.

## 16. GDPR / EU data handling

- Mail content is personal data; with this design the operator processes only
  ciphertext plus account/billing identity and connection metadata. The
  operator is still a data controller for account/billing data and processor
  (at most) for ciphertext. **[inferred; not legal advice]**
- Required before launch: EU data residency for storage and backups, a data
  processing agreement template, a published retention table (section 14), a
  records-of-processing entry, a DPIA (recommended given the data class), and a
  72-hour personal-data breach notification runbook. Theft of encrypted blobs
  with keys unaffected lowers the risk and may remove the duty to inform the
  affected users; whether the supervisory authority must still be notified is
  a separate risk assessment, and account/metadata exposure counts on its
  own. Get this reasoning reviewed by counsel and written down in advance,
  don't improvise it during an incident. **[inferred; not legal advice]**
- Deletion: deleting a user's envelopes, wrapped keys, queued Send Later jobs
  and account row removes everything the service holds about them, and what
  remains in backups is unreadable without the client-held keys. It does not
  delete their mail, which the service never held: that stays at the provider
  and on their devices. Say so in the erasure response. **[inferred]**

## 17. Abuse and spam-relay risk

- With client-side sending (section 10), the service never originates mail:
  no relay to abuse. **[inferred]**
- Remaining abuse: storage abuse (storing non-mail ciphertext), credential
  stuffing against tokens, free-tier exploitation. Mitigations: envelope size
  classes and per-tenant quotas, token rate limits, paid/verified accounts for
  large storage, an abuse contact and suspension process. If helper-side Send
  Later is kept, it is a sending path: per-tenant rate limits, DKIM/SPF
  alignment with the user's own provider, and immediate suspension tooling are
  mandatory.

## 18. Audit logging without content

- Server-side, append-only, tamper-evident log (hash-chained) of: auth events,
  token issue/revoke, admin actions, enrolment/revocation events, quota
  changes. Log object IDs and counts, never content, never tokens, never IP +
  content correlation. **[inferred; standard practice]**
- Admin actions require multi-person approval where feasible (two-person rule
  for production data access), so "admin can't reach inside" is procedural as
  well as cryptographic.

## 19. Incident response and assurance program

- Incident response: severity ladder, on-call, the GDPR runbook (section 16),
  user-notification templates for availability vs confidentiality events.
- Independent security review BEFORE launch: cryptographic design review
  (envelope format, hybrid combiners, enrolment ceremony) and a penetration
  test of the deployed service. Repeat annually and on major crypto changes.
- Bug bounty or coordinated vulnerability disclosure from day one (a published
  security.txt and disclosure policy is the minimum; a paid bounty once revenue
  supports it). **[inferred]**
- Compliance path if B2B matters later: SOC 2 Type II / ISO 27001. Not a
  substitute for the crypto audit.
- Publish the threat model (section 3) and the claim with its caveats
  (section 1). The claim is only defensible if it is public and specific.

## 20. Migration from the current helper

1. Current single-user helper unchanged; Studio deploy continues in
   Send Later-only mode.
2. Make the mailbox-service mode **opt-in** (off by default) in new installs,
   with in-app copy explaining that it stores mail on the server; mark it for
   deprecation in ROADMAP/release notes.
3. Enrol the owner's devices (dogfood) for the Send Later helper and, if
   wanted, selective device-transfer envelopes.
4. Give mailbox-service users an export-and-delete path (the client already
   holds a local copy). The helper's mailbox database is unencrypted, so
   there is no key to destroy: finishing means deleting the database, its
   WAL and every backup of it.
5. Cut over per account; retire the mailbox API when usage reaches zero; the
   helper continues in Send Later-only mode (`--no-sync`, documented
   `docs/SEND_LATER.md:90`).

## 21. Phased plan and effort (repo scale: Easy/Medium/Large)

| Phase | Content | Effort | Buys |
| --- | --- | --- | --- |
| P0 | Harden current deploy (TLS 1.3-only on both the mailbox API and the Send Later listener, tailnet-only, ACLs, FileVault, least-privilege) | Easy | Transport hybrid-PQ; host exposure down |
| P1 | Send Later hybrid-PQ + FS + rotation (if helper delivery kept) | Medium | E2EE+PQ for scheduled outgoing |
| P2 | Optional selective device-transfer envelopes (TTL, user-chosen scope) + enrolment | Large | Cross-device catch-up without a server-held mailbox |
| P3 | Recovery phrase + revocation | Medium | Device lifecycle, operator-proof |
| P4 | Verifiable client builds: reproducible builds from public source, published verification steps | Medium-Large | The claim extends from "server" to "operator" (section 2, item 3) |
| P5 | Multi-tenant hardening: isolation, quotas, abuse, audit log, retention | Large | Hosted-service readiness |
| P6 | Assurance: external crypto audit, pentest, disclosure program, GDPR pack | Medium (calendar-heavy) | The claim becomes defensible |
| P7 | Migration + mailbox-API retirement | Medium | Completes cutover |

Effort ratings are inferred from the repo's scale and named surfaces, not
measured velocity.

## 22. Decisions the owner must make

1. Accept that **server-side sync/filters/Gmail push cannot exist** in this
   design (clients sync when open).
2. **Send Later**: helper delivery (the server then holds sending credentials
   and sees scheduled mail at delivery; section 10), client-side delivery
   with scheduling degradation, or drop it from the hosted service.
   Interim choice (2026-11-11): helper delivery on the single-user Studio in
   Send Later-only mode is accepted now; the hosted service must revisit this
   before multi-tenant launch.
3. Decided 2026-11-11: **native clients only, no web client** (an
   operator-served web app would defeat operator-proofing; section 2, item 3).
4. Decided 2026-11-11: **the server does not hold the mailbox**; mailbox-service
   mode goes opt-in now, deprecated later (section 20).
5. Recovery: offline phrase, escrow, or both.
6. **Verifiable builds**: make reproducible, independently verifiable client
   builds a launch requirement, or launch with the narrower claim "the server
   cannot read mail" (section 2, item 3).
7. Fund the external audit + CASA verification + legal (GDPR pack) before
   launch; the zero-knowledge claim should not ship unaudited.
8. Launch geography: EU-only residency at first?
9. **In-flight mailbox-service work**: the open `HOSTED-SERVER-*` tasks in
   TASKS.md build on the helper holding mailbox data. Finish them for the
   single-user helper, or freeze them now that the mode is on a deprecation
   path?

Open: 1, 2 (for the hosted service), 5-9. Decided: 3, 4.

## 23. Expert review flags (collected)

- Envelope format v1 (section 8).
- Hybrid KEM combiner, Send Later and device wrap (sections 7, 10).
- Enrolment ceremony, signed device list and UMK rotation order (section
  12), recovery-key derivation (section 13).
- Threat-model completeness for a paid multi-tenant service (section 3),
  including the operator-as-publisher update path (T6).
- Credential handling for helper-side Send Later, if kept (section 10).
