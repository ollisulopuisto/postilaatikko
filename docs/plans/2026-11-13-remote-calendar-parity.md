# Remote Calendar parity

Status: implementation in progress, not usable remote parity.

The trusted mailbox helper should own Calendar stores, credentials, provider synchronization, mutation receipts and pending writes. The desktop owns calendar presentation, device display preferences and interactive authorization UI. This is the owner-trusted helper, not the hosted zero-knowledge service.

Use calendar.read for calendar metadata/event reads, calendar.write for event mutations and RSVP, and calendar.manage for source setup, credentials, synchronization controls and consent. Mail capabilities alone grant none of these. Never serialize credential-store adapters or return tokens/passwords in metadata or errors. A remote HTTP caller cannot select the extension manifest, credential namespace or host-secret class.

Reuse the compiled Calendar backend and the shared hostaccess adapters. Contacts owns the headless Core adapter; Calendar must use it rather than reimplement token refresh. The helper needs explicit startup/shutdown of its Calendar Store, Syncer, PendingQueue and alarm scheduler. A desktop disconnect must not stop helper synchronization, and helper shutdown must cancel provider requests and wait for workers before closing stores.

Expose typed operation routes, not reflection or an arbitrary Wails method dispatcher. Bound request bodies, identifiers, time windows, calendar/attendee lists, recurrence expansion, response size and concurrency before provider/store work. Validate Calendar ownership in the extension service. Treat HTML as untrusted and preserve existing sanitization.

Event creation/updates/deletion, RSVP and source provisioning need durable request IDs and request fingerprints scoped to the authenticated device. Repeated calls replay confirmed results, never provider mutations. Interrupted or ambiguous operations remain unknown, including a helper crash after provider acceptance; do not automatically resend invitations. Existing provider revision/ETag conflicts must remain visible.

Interactive OAuth consent cannot run unattended on the helper. Use an authenticated, expiring device-mediated authorization transaction with explicit account identity verification, cancellation and scoped callback handoff. Do not open a helper-local browser or copy existing desktop credentials on connect. CalDAV source setup transmits operator-entered credentials only through the trusted TLS API under a management grant, with no request logging; no source-provisioning operation should be added until retries and network-destination policy are defined.

Alarm notifications need device-aware replay/acknowledgment so a remote helper does not mark a reminder delivered merely because its own desktop notification API is unavailable. Define one owner of dismiss/snooze state, bound event retention and ensure reconnect/restart do not duplicate alerts.

Implementation sequence: shared host access; bounded read API and routing; helper Calendar lifecycle; event mutation receipts and routing; source/consent management; notifications and parity acceptance. Keep unsupported paths explicit until their implementation exists. Do not remove the global remote gate and let calls fall through to missing local stores.

Red-first tests must cover missing capabilities, malformed/unbounded input, cross-source targets, recurrence bounds, duplicate requests, provider acceptance with lost acknowledgement, offline/restart, token expiry/revocation, Calendar disabled state, and remote desktop startup without a mailbox database. Native two-machine, real CalDAV/Google/Microsoft authorization, recurrence/RSVP/invitations, alarms through sleep/reboot and actual CPU/RSS comparison remain acceptance work.
