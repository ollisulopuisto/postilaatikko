# Native Gmail filter review

This UI is a separate provider-native workflow in Filters settings. It does not translate local substring conditions, copy local priority, or apply a new rule to historical mail.

## Entry and limits

Stop a single-account local rule with no folder scope or move destination, then choose its Gmail review action. Backend validation requires an enabled Google account on the captured execution owner and no outstanding automatic jobs. Explicit standalone execution uses local credentials; configured helper execution uses only helper credentials, without an offline fallback.

The selected account requests Gmail settings consent only through the explicit authorization button. Existing granted scopes and account identity remain verified by the consent prerequisite.

## Review

Write an explicit Gmail query and choose supported label actions: mark read, archive or Trash, and selected account user labels. System labels cannot be supplied through the user-label list. Archive and Trash cannot both be chosen.

Preview reads one provider page, at most 51 message IDs. The UI shows at most 50 plus a truncation marker, never the provider's approximate total. The preview is read-only and is not a guarantee about future delivery. Editing the query/actions invalidates the displayed preview.

Review binds a fingerprint to the exact account, stopped source snapshot and native criteria/actions. The transactional preparation step freezes this source and checks the frozen fingerprint again. The next screen shows the frozen query/actions and requires a separate Publish to Gmail click.

## Recovery

The durable journal owns the one-create-attempt boundary. A create error reloads that state before any further action is offered. Attempted/unknown results expose read-only Check Gmail result, never another create. Closing/reopening the dialog recovers its account-qualified journal entry. Only an unattempted prepared review may be cancelled to release its local source. A verified publication keeps the source guard; reverse transfer and provider-filter removal are not implemented.

## Verification

Backend tests cover bounded provider requests, no writes during preview/review, stale fingerprints, selected-account user labels, preview failure without reservation, restart readback, and canonical fingerprint persistence with empty/duplicate label lists. The actual browser fixture covers selected consent, failed preview, preview invalidation, final review, cancellation, uncertain create, reopen and read-only recovery, including English and Finnish narrow layout captures. Live Google consent/provider matching and native macOS execution still require acceptance testing.

## Durable provider owner binding

The preview and prepared journal also bind the normalized Gmail account email and resolved Mail OAuth client ID, stored as non-secret metadata in the existing source-snapshot JSON. An account/project change invalidates review. The UI displays the email from the actual preview/frozen plan, not a stale account-context label.

Before provider catalog reads, creation or reconciliation, the adapter verifies the authenticated Gmail profile. One token is pinned across that identity read and the following provider operation, so a credential replacement cannot switch the write's mailbox between those requests. A stale/expired pinned token fails rather than refreshing or retrying the write. No token is persisted in the publication journal.

Old unbound prepared plans must be cancelled and reviewed again. An attempted old unbound plan needs manual inspection; it cannot be replayed or automatically reconciled against a guessed account. A configured mailbox-service connector blocks standalone consent. Its selected helper instead owns the separate consent lifecycle described below.

## Configured helper workspace

The same dialog now captures the execution owner and client before review and displays its endpoint. Helper context, consent, preview and durable recovery use only that helper. Connector, endpoint or credential replacement invalidates the open review. Standalone paths remain explicit and never serve as an offline fallback. See GMAIL_FILTER_HELPER.md for the shipped routing, consent and validation boundaries. Earlier standalone-only notes above describe the initial prerequisite, not the current helper support.
