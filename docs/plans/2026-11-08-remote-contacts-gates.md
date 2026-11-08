# Remote contacts and CardDAV feature gates

**Goal:** Make local-only Contacts, CardDAV and contact OAuth entry points fail closed while the desktop is connected to a remote mailbox helper.

## Scope

- Guard Wails methods for contact search/read/write/list/photo lookup, CardDAV discovery and source/addressbook management/sync, contact-source OAuth setup/reauthorization, and the embedded Contacts extension bridge.
- Reject remote-mode calls before using local mailbox stores, local credentials, or local OAuth state.
- Return explicit errors that identify the unavailable remote feature; preserve local-helper behavior.
- Do not add helper APIs, remote contact storage, UI redesign, crypto gates, or mailbox database changes.

## Acceptance

- Every exported Contacts/CardDAV/contact OAuth method that requires local-only state checks remote mode before accessing that state.
- Every promoted `Contacts_*` bridge method is overridden by an App wrapper that checks remote mode before bridge dispatch, so skipped remote bridge initialization cannot panic and remote calls cannot report empty success.
- Remote-mode calls return an explicit unsupported-mode error and do not panic or report successful empty data.
- Local-mode code paths retain their existing behavior.
- The affected app and helper packages build and `git diff --check` passes.
- No tests are added or run for this slice.

## Verification limits

- Remote Contacts/CardDAV functionality remains unavailable until helper-owned APIs are designed and implemented.
- Frontend presentation of unsupported feature errors and remote crypto operations remain separate follow-up work.
