# Roadmap

Planned, in flight, open questions, and parked items for this fork. Done items are marked with the version that shipped them. Suitable changes are intended to be proposed upstream to Eterno Mail after they are stable.

## Product direction

The goal is a strong personal email client for people who use several accounts. Spark and Gmail are references for useful interaction patterns, with the focus kept on one person's mail and workflows.

### Priorities

1. **Scheduled send — critical.** Compose now, choose a delivery time, and have Postilaatikko send it reliably.
2. **Gmail API integration.** Add server-side Gmail search and Gmail-native capabilities for Gmail accounts, starting with spam actions and efficient mailbox updates; keep IMAP support for other providers.
3. **Calendar from email:** recognize date/time mentions and offer links that open or create events in installed calendar applications.
4. **Fast everyday mail:** Gmail-style keyboard bindings, quick reply templates, and full email templates.
5. **Better compose controls:** use existing cross-account sender identities with signatures selected from sender, recipient, and other available context.
6. **Cross-account automation:** local rules can route incoming mail across accounts when provider-side filters cannot.
7. **Multiple mail windows:** keep several mail views open for parallel reading and searching. A detached compose window already exists; this goal is multiple mail windows.
8. **Smart notifications:** optional sender- and rule-based controls. Useful, but lower priority than the items above.

### Product boundaries

- This is a personal mail client. Shared inboxes, team assignment, and collaboration features are out of scope.
- Prefer local cross-account behavior when a provider API cannot perform the action, and never delete a source message until a cross-account copy has been confirmed.
- Keep notifications configurable and avoid making them a requirement for the core mail workflow.
- Prefer contextual action surfaces (selection bar, context menus, selection popovers) over a floating or dockable tool palette. All of them, and the command panel, draw on one shared action definition list tied to the shortcut registry. See `docs/plans/2026-10-25-contextual-toolbars-design.md` (D-001; follow-ups P-029 to P-034).

### Recommended implementation order

1. **ICS event handoff (quick win).** Given a structured event, create a standard ICS file and open it for review/import in the user's calendar application.
2. **Scheduled send (critical).** Start with the local helper on the always-on Mac Studio. Build end-to-end encryption into the first version: devices encrypt queued messages for the paired helper, and any relay stores only ciphertext. The helper decrypts locally at send time; its private key and account credentials stay on the Studio. A future paid hosted sender would be a separate trust mode, not E2EE.
4. **Gmail API integration.** Establish the Gmail-specific authorization and provider path, then use it for server-side search and Gmail-native capabilities. Keep the existing IMAP path for non-Gmail accounts and use the least-permissive scopes that cover each operation.
5. **Date/time recognition in email.** Detect likely event times and connect them to the ICS event handoff, asking for confirmation when timezone or event details are ambiguous.
6. **Gmail-native spam actions.** Mark as spam and undo/report as not spam using Gmail's own mailbox labels and behavior, through the Gmail API.
7. **Gmail-style keyboard bindings.** Add Gmail-like message-list and message-reading actions. Keep single-key bindings inactive while composing or typing in other editable fields.
8. **Local cross-account filters.** The cross-account move action already exists, so rules can build on it without inventing a second move path.
9. **Contextual signatures.** Extend the existing cross-account identities and per-identity signatures with sender/recipient/message-type rules.
10. **Reply and email templates.** Build these on top of the compose identity and signature behavior.
11. **Multiple mail windows.** Plan around the existing detached composer, but support multiple mail views and their separate navigation state.
12. **Smart notifications.** Add optional per-sender and rule-based notification controls after the core workflows.

## Guidance for agents

Read this before writing code in this repo. Full contributor rules are in `CONTRIBUTING.md`; these are the ones that matter most here.

- **Test first (red, then green).** For every bug fix or feature, write a test that reproduces it, run it and confirm it fails, then implement and confirm it passes. Do not fix and move on without a regression test. If the project lacks test infrastructure for a case, add it rather than skip the test.
- **Go tests are table-driven** where several cases share a shape. Run `go test ./...` (or `make test`, which sets the WebKit build tags) before committing.
- **Go style:** `gofmt`, explicit error handling with no silently dropped errors, zerolog for logging, a comment on every exported function.
- **Control flow:** avoid `else` and `else if` chains. Use guard clauses with early returns, or a `switch`.
- **Keep the CHANGELOG current.** Each commit that changes behaviour adds its entry under the version heading. Write it for whoever promotes the release, in terms of what changes for users or operators, not file names.
- **Keep this roadmap current in the same commit as the work.** Mark items done with the version, and list anything unfinished, untested or waiting on a human.
- **Commit messages** use the `feat:`, `fix:`, `docs:`, `refactor:`, `test:` or `chore:` prefixes from `CONTRIBUTING.md`. Commit locally; push only when asked.
- **Ask before deciding things that are the user's to decide** (e.g. the open questions below). Choose sensible defaults for everything else and say what you chose.

## PR-sized tasks

These are deliberately small, reviewable candidates that can be offered upstream independently. The current in-flight fixes are not complete until their changes are verified and recorded here.

### Easy

- **Finnish UI localization.** Add the core and extension `fi` dictionaries, register the locale, and verify key and placeholder parity. In progress via AGY CLI.
- **Use account display names in the sidebar.** Prefer the configured name under Inbox and in account rows, falling back to the email address. In progress.
- **Keep Edit Account dialog geometry stable.** Anchor the footer and keep tab content in a bounded scrolling region so changing tabs cannot move the Save/Close controls. In progress.
- **macOS window conventions.** Add `⌘,` for Preferences and title-bar double-click maximize/restore. In progress; the Postilaatikko title string itself is fork-specific.

### Medium

- **Header-only routine sync.** Stop automatic full-body and attachment downloads; fetch content when a message is opened. In progress after user confirmation.
- **Sidebar folder access.** Keep the all-folder browser directly reachable and add pinned folders, then persist item order and visibility.
- **Virtual-folder criteria.** Add include/exclude folders, read/unread and replied/unreplied status, and 7-day / 30-day / all-time windows.
- **macOS Contacts source.** Start with read-only browsing and recipient autocomplete. Offer a link/action to open the selected person in Apple's Contacts app for modifications; Postilaatikko will not write contact changes in the first version.

### Large

- **Gmail API search.** Search Gmail online with Gmail's query syntax while preserving the existing IMAP path for other accounts.
- **Scheduled send.** Durable E2EE queue plus the always-on Mac Studio helper, retries, cancellation, and delivery status. The helper is the trusted endpoint; a future paid hosted sender needs a separate, clearly disclosed trust model.
- **Calendar links from email.** Create a standard ICS event from a structured time, then recognize dates and times in email and offer the event for review/import in the installed calendar app.
- **Multiple mail windows.** Support independent mail views and navigation state beyond the existing detached composer.

## Must have

- **Gmail API integration.** Not started. Use Gmail's API for server-side search and Gmail-specific capabilities such as native spam actions, filters, aliases/signatures, and mailbox change notifications. Preserve IMAP support for non-Gmail providers and avoid replacing working provider-neutral paths without a clear benefit. Request only the OAuth scopes needed for enabled features. See the [Gmail API overview](https://developers.google.com/workspace/gmail/api/guides) and [scope guide](https://developers.google.com/workspace/gmail/api/auth/scopes).
- **Scheduled send (critical).** Compose and send at a chosen time through the trusted always-on Mac Studio helper. P-011 now has a current-main review milestone with durable E2EE client/helper queues, encrypted acknowledgements/status, local pairing permissions, login-agent setup, main/detached composer scheduling, revision-based time changes, a paged Scheduled view, delivery-confirmed guarded draft cleanup, queue limits, and atomic Send Now/schedule arbitration. Private keys and mail credentials stay on Studio; ambiguous SMTP outcomes require inspection and never blindly resend. The Scheduled sidebar entry awaits integration with the active sidebar work. Live delivery, keychain/login lifecycle, and security/reliability regression gates remain unverified, so this is not Done or released. Relay transport, key rotation, and explicit history retention remain follow-ups. A future paid hosted sender is a separate mode and cannot be represented as E2EE if the service must decrypt and send.
- **Unified virtual folders (v0.3.10; criteria expanded in v0.3.15).** Saved sets of real folders across accounts, shown in the sidebar. Base folder-set behavior implemented in v0.3.10; criteria facets (include/exclude, read/unread, starred/unstarred, replied/unreplied, date windows) implemented in v0.3.15.
- **Fast folder browser and pins.** Keep a fast browser for all account folders available from the sidebar and let users pin selected folders for one-click access. The current full folder tree is in the temporary More panel; there is no pinned-folder list.
- **Sidebar customization.** Let users rearrange sidebar items and hide selected items, persisting the layout across launches. Virtual folders can currently be reordered among themselves; other sidebar sections and folders cannot be rearranged or hidden (collapsing a tree is not the same as hiding it).
- **Header-only bulk sync.** Make “Sync all” fetch folder/message headers without downloading message bodies or attachments. Fetch full content on demand when a message is opened. Current `SyncAllComplete` calls `SyncFolder`, which starts a background body-and-attachment fetch for missing bodies in each account's sync window.
- **Cross-account message moves.** Already supported. Preserve the safety rule: confirm the destination copy before removing the source.
- **Multiple sender identities across accounts.** Already supported in the composer. Improve it with contextual signature selection (sender, recipient, and message type); signatures can already be configured per identity.
- **Local cross-account filters.** Not started. Build on the existing cross-account move action; match incoming messages and route them to a folder in another account.
- **Gmail-style keyboard bindings.** Fast list navigation and common message actions while browsing lists and reading messages, documented in `docs/KEYBOARD_SHORTCUTS.md`. Disable single-key bindings in compose and other editable fields; keep standard macOS editing shortcuts working. Not started.
- **Gmail-native spam actions.** Let users mark messages as spam and undo that action for Gmail accounts, invoking Gmail's native spam classification/disposition rather than only moving mail to a local folder. The Gmail API supports applying/removing the `SPAM` system label; confirm the exact classifier-training behavior during implementation. Not started.
- **Quick replies and email templates.** Save reusable short replies and complete message templates as separate composer tools. Preserve the current sender identity and draft fields when inserting either. Not started.
- **Calendar recognition.** Turn recognized date/time mentions in message content into an “Add to Calendar” action using the standard ICS event handoff. Open it for review/import and handle timezone ambiguity conservatively. Not started; build on `extensions/calendar`.
- **Multiple mail windows.** Keep more than one mail window open for parallel reading and searching. Not started; the detached compose window already exists.

## Planned

- **macOS Contacts integration.** Start with read-only address book browsing and recipient lookup/autocomplete, with macOS privacy authorization. For edits, link to the selected person in Apple's Contacts app; do not write contact changes from Postilaatikko in the initial version.
- **Smart notifications.** Per-sender and rule-based controls for deciding which messages produce notifications. Not started; lower priority.
- **Snooze and reminders.** Hide or remind about a conversation at a chosen time. Not started and lower priority than the core items above.

## Open questions

- **Paid hosted scheduled send.** It may serve users without an always-on computer, but the service would have to access message content and sending credentials at delivery time. Decide whether to offer this separate, non-E2EE trust mode; keep the initial Mac Studio helper mode E2EE.
- **Gmail-style key map.** Start with message-list and reading actions (navigation, archive, select, star, move, delete, spam, reply/forward, undo); single-key shortcuts are inactive in compose/editable fields. Refine exact bindings during implementation.

## Parked

- Nothing parked right now.

## Done

- **Virtual-folder criteria and default sync (v0.3.15).** Saved folder sets expanded into full cross-account views with include/exclude folder selection, read/unread, starred/unstarred, replied/unreplied, and date window (7d, 30d, 90d, 1y, all-time) criteria. Removed hardcoded unread-only constraint; criteria apply consistently across conversation lists and in-folder search. Default sync period updated to All Messages (0 days) across models, OAuth, and UI, with migration 52 upgrading legacy defaults.
- **Command palette and Gmail shortcuts (v0.3.14).** Alfred-style quick launcher (`⌘P` / `Ctrl+P`), Gmail standard shortcuts (`v`, `e`, `c`, `r`, `a`, `f`, `x`, `/`, `!`, `#`, `z`, `Shift+I`, `Shift+U`, `o`, `u`), shortcut dialog layout stabilization, configurable mark-as-read delay, sidebar subfolder tree navigation, and i18n cleanup.
- **Keyboard shortcut overlay and discoverability (v0.3.13).** Unified keyboard shortcut registry, searchable `?` reference dialog, and UI tooltips and key hints.
- **Gmail API integration (v0.3.13).** Server-side search using Gmail query syntax and native mark-as-spam / not-spam actions through the Gmail API, with graceful fallback to IMAP.
- **Composer quick replies (v0.3.13).** Saved snippets managed from the composer footer without mutating draft state.
- **Local cross-account filters (v0.3.13).** Automatic routing of incoming mail across accounts based on rules, using confirmed copy before source deletion.
- **ICS event handoff (v0.3.13).** Standard `.ics` event creation and handoff to system calendar.
- **Accessibility and localization (v0.3.13).** Localized accessible names across Contacts and shared UI kit controls.
- **Routine header-only IMAP sync (v0.3.12).** Body content loaded on demand.
- **Finnish localization and macOS window conventions (v0.3.12).** Finnish translations, `⌘,` shortcut, title-bar maximize.
- **Unified virtual folders (v0.3.10).** Saved views across selected folders and accounts, including sidebar management, message listing, and unread counts.
- **Postilaatikko product naming (v0.3.10).** Updated visible product branding while retaining compatibility-sensitive app identifiers.
