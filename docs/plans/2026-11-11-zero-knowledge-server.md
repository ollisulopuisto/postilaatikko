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
- **Scheduled sending has a delivery-window carve-out** unless sending moves to
  the client. (Section 10.)
- The claim holds only for **native clients**. A web client served by the
  operator defeats it (T6).

## 2. What cannot be met, stated up front

1. **Server-side incoming sync is incompatible with zero knowledge.** Syncing
   IMAP/Gmail requires the user's credentials and processes plaintext. The
   current helper does exactly this **[documented: docs/SEND_LATER.md:71,88]**.
   In this design, sync happens on client devices only; the server never sees
   provider credentials.
2. **A malicious operator cannot be stopped from attacking availability.**
   Deleting blobs, refusing service, or rolling back state are outside what
   cryptography prevents. Mitigations are backups the user can verify and
   export, and contractual/legal exposure for the operator. **[inferred]**
3. **A web client served by the operator cannot be trusted.** An operator (or
   whoever compromises the web deployment) can ship JavaScript that exfiltrates
   keys or plaintext. Zero knowledge to the operator requires signed, native
   client applications distributed through channels the operator does not
   control unilaterally (App Store / direct signed builds with reproducible
   build verification as a stretch goal). **[inferred; standard argument in the
   E2EE industry]**
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
| T2 | Malicious or compelled operator (root on everything) | Same as T1, because keys never leave clients | Metadata; availability; can serve evil web client (so: native clients only) |
| T3 | Malicious tenant | Other tenants' content (per-user keys, tenant isolation, section 6) | Shared-infrastructure metadata (timing on shared queues) **[inferred]** |
| T4 | Network attacker (incl. on the tailnet/ISP path) | Content (app-layer E2EE) and recorded-traffic decryption (hybrid PQ TLS) | Traffic analysis |
| T5 | Stolen/lost endpoint | Locked device with FileVault/keychain protects at rest; revocation (section 12) limits a live theft | An unlocked, logged-in device exposes that user's mail |
| T6 | Malicious client update | Out of scope for native signed builds with reproducible/verified builds; unfixable for operator-served web | Web clients: not protected |

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
2. **The server is limited to what is strictly needed**: the Send Later relay
   (already E2EE), and optionally small, selective encrypted transfer between a
   user's own devices - never a full server-held archive.
3. **The current mailbox-service mode moves to opt-out, then deprecated**
   (section 20): it is the mode where the server holds mail, which this design
   retires.

**Send Later relay (existing, upgraded cryptography, section 10).** The one
standing server role. Holds only encrypted scheduled-outgoing envelopes.

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
| Send Later-only | encrypted outgoing envelopes + tombstones, capped **[documented: 10,000 jobs / 512 MiB, docs/SEND_LATER.md:63]** | tens of MB typical; 512 MiB ceiling |
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

- **Master Mailbox Key (MMK)**: 256-bit, generated on the user's first enrolled
  client (CSPRNG). Never leaves clients unwrapped.
- **Per-message content keys (CEK)**: random 256-bit; XChaCha20-Poly1305 for
  message ciphertext (192-bit nonces, safe random); CEK wrapped to MMK;
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

Self-describing envelope per message version: header (version, namespace,
message UID, CEK wrap, timestamps) + ciphertext; AAD binds namespace, folder,
UIDVALIDITY, UID. **Expert review flag** before implementation. **[inferred]**

## 9. Transport security

- TLS 1.3 only (`tls.VersionTLS13`, one-line change at
  `internal/automation/server.go:178`) **[documented location; inferred
  effect]**.
- Hybrid post-quantum key exchange X25519MLKEM768 is Go's TLS 1.3 default since
  Go 1.24 **[inferred: Go release notes]**; Go clients and server get it free.
  Non-Go clients (future mobile) need per-platform verification.
- App-layer E2EE makes TLS defense in depth, not the boundary.

## 10. Sending and Send Later under the headline requirement

- **Immediate sending**: client-side, user's own SMTP credentials, never
  touching the service. No relay, no abuse surface (section 17). **[inferred]**
- **Send Later**: someone must hold plaintext MIME at delivery. Options:
  (a) helper-side delivery (current design, upgraded crypto below) - a live
  operator/compromise at delivery time can read that one payload; violates the
  headline for scheduled mail; (b) client-side delivery when any enrolled
  client is online, else the job waits - meets the headline, degrades
  scheduling reliability; (c) drop Send Later from the hosted service.
  **Owner decision (section 19).**
- If helper-side delivery is kept, upgrade its crypto: ephemeral per-submission
  hybrid X25519+ML-KEM-768 KEM (X-Wing construction or HPKE hybrid DHKEM;
  **expert review flag**), rotating helper KEM keys (rotation already needed
  **[documented: docs/SEND_LATER.md:116]**), per-job key deletion on
  completion for forward secrecy, helper receipts signed with the service
  identity key.

## 11. Search over encrypted data

| Option | Server learns | Effort | Notes |
| --- | --- | --- | --- |
| S1. Client-side search only | nothing about queries/content | Small | Decrypt into the client-local DB, search with existing FTS **[documented: messages_fts, internal/database/migrations.go:413]**. Server sees access patterns only. Recommended. |
| S2. Deterministic keyword tokens (e.g. AES-SIV) | keyword frequency/equality; repeated queries | Medium | Frequency-analysis attacks known. Not recommended. |
| S3. Structured searchable encryption (OXT-style) | formal leakage profile | Large | **Expert review mandatory.** Evaluate only if archives outgrow client indexing. |

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
  authentication string out-of-band, wraps the MMK to the new device's hybrid
  KEM public key, uploads the wrapped blob. **Expert review flag** on the
  ceremony (compare published Signal/1Password models). **[inferred]**
- Revoke: delete the device's wrapped-MMK blob, revoke its token (documented
  `api-client revoke` machinery as the pattern), re-key forward (new MMK for
  new mail). Full history re-encryption optional and expensive. A live stolen
  unlocked device exposed what it read (T5).
- Lost last device = recovery phrase only; no administrative backdoor, by
  design. Tell users this in plain words before they enable it.

## 13. Backup and recovery

- Recovery phrase: 256-bit seed as a standard word-list phrase or base32,
  written down / password-manager escrow; optional second escrow (e.g. iCloud
  Keychain). **Owner decision.**
- Server backups are ciphertext: safe by construction. Keep Send Later
  tombstones **[documented: docs/SEND_LATER.md:116]**.
- User-verifiable export: clients can always re-download all envelopes and
  decrypt locally - exit is not hostage to the operator (mitigates T2
  availability partially). **[inferred]**

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
  third-party-access obligations. **[inferred]**
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
  72-hour personal-data breach notification runbook (encrypted-blob theft is
  likely not a notifiable personal-data breach if keys are unaffected -
  document the reasoning, don't improvise it during an incident). **[inferred]**
- Deletion: deleting a user's blobs + wrapped keys + account row is real
  deletion of their mail (crypto-shredding is inherent: without keys, blobs are
  noise). Honour erasure requests by destroying both. **[inferred]**

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
3. Enrol the owner's devices (dogfood) for the Send Later relay and, if
   wanted, selective device-transfer envelopes.
4. Give mailbox-service users an export-and-delete path (the client already
   holds a local copy; server-side deletion plus key destruction finishes it).
5. Cut over per account; retire the mailbox API when usage reaches zero; the
   helper continues in Send Later-only mode (`--no-sync`, documented
   `docs/SEND_LATER.md:90`).

## 21. Phased plan and effort (repo scale: Easy/Medium/Large)

| Phase | Content | Effort | Buys |
| --- | --- | --- | --- |
| P0 | Harden current deploy (TLS 1.3-only, tailnet-only, ACLs, FileVault, least-privilege) | Easy | Transport hybrid-PQ; host exposure down |
| P1 | Send Later hybrid-PQ + FS + rotation (if helper delivery kept) | Medium | E2EE+PQ for scheduled outgoing |
| P2 | Optional selective device-transfer envelopes (TTL, user-chosen scope) + enrolment | Large | Cross-device catch-up without a server-held mailbox |
| P3 | Recovery phrase + revocation | Medium | Device lifecycle, operator-proof |
| P4 | Search beyond S1 (optional) | Large + review | Server-side queries, bounded leakage |
| P5 | Multi-tenant hardening: isolation, quotas, abuse, audit log, retention | Large | Hosted-service readiness |
| P6 | Assurance: external crypto audit, pentest, disclosure program, GDPR pack | Medium (calendar-heavy) | The claim becomes defensible |
| P7 | Migration + mailbox-API retirement | Medium | Completes cutover |

Effort ratings are inferred from the repo's scale and named surfaces, not
measured velocity.

## 22. Decisions the owner must make

1. Accept that **server-side sync/filters/Gmail push cannot exist** in this
   design (clients sync when open).
2. **Send Later**: helper delivery with the live-window carve-out, client-side
   delivery with scheduling degradation, or drop it from the hosted service.
   Interim choice (2026-11-11): helper delivery on the single-user Studio in
   Send Later-only mode is accepted now; the hosted service must revisit this
   before multi-tenant launch.
3. Decided 2026-11-11: **native clients only, no web client** (an
   operator-served web app would defeat operator-proofing; section 2, item 3).
4. Decided 2026-11-11: **the server does not hold the mailbox**; mailbox-service
   mode goes opt-in now, deprecated later (section 20).
4. Recovery: offline phrase, escrow, or both.
5. Revocation: forward-only re-key, or full history re-encryption on device
   compromise.
6. Fund the external audit + CASA verification + legal (GDPR pack) before
   launch; the zero-knowledge claim should not ship unaudited.
7. Launch geography: EU-only residency at first?

## 23. Expert review flags (collected)

- Envelope format v1 (section 8).
- Hybrid KEM combiner, Send Later and device wrap (sections 7, 10).
- Enrolment ceremony (section 12).
- Any S3 searchable-encryption work (section 11).
- Threat-model completeness for a paid multi-tenant service (section 3).
