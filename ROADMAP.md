# Roadmap

Planned, in flight, open questions, and parked items for this fork. Done items are marked with the version that shipped them. Suitable changes are intended to be proposed upstream to Eterno Mail after they are stable.

## Product direction

The goal is a strong personal email client for people who use several accounts. Spark and Gmail are references for useful interaction patterns, with the focus kept on one person's mail and workflows.

### Priorities

1. **Scheduled send — critical.** Compose now, choose a delivery time, and have Postilaatikko send it reliably.
2. **Gmail API integration.** Add Gmail-native capabilities for Gmail accounts, starting with spam actions and efficient mailbox updates; keep IMAP support for other providers.
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

### Recommended implementation order

1. **Scheduled send.** The user has identified this as critical; use a small local helper on their always-on Mac Studio for the initial personal setup.
2. **Gmail API integration.** Establish the Gmail-specific authorization and provider path, then use it for Gmail-native capabilities. Keep the existing IMAP path for non-Gmail accounts and use the least-permissive scopes that cover each operation.
3. **Calendar recognition.** Add date/time links to messages and connect them to the calendar extension or installed calendar applications.
4. **Gmail-native spam actions.** Mark as spam and undo/report as not spam using Gmail's own mailbox labels and behavior, through the Gmail API.
5. **Gmail-style keyboard bindings.** A contained, independent improvement to everyday navigation.
6. **Local cross-account filters.** The cross-account move action already exists, so rules can build on it without inventing a second move path.
7. **Contextual signatures.** Extend the existing cross-account identities and per-identity signatures with sender/recipient/message-type rules.
8. **Reply and email templates.** Build these on top of the compose identity and signature behavior.
9. **Multiple mail windows.** Plan around the existing detached composer, but support multiple mail views and their separate navigation state.
10. **Smart notifications.** Add optional per-sender and rule-based notification controls after the core workflows.

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

## Must have

- **Gmail API integration.** Not started. Use Gmail's API for Gmail-specific capabilities such as native spam actions, filters, aliases/signatures, and mailbox change notifications. Preserve IMAP support for non-Gmail providers and avoid replacing working provider-neutral paths without a clear benefit. Request only the OAuth scopes needed for enabled features. See the [Gmail API overview](https://developers.google.com/workspace/gmail/api/guides) and [scope guide](https://developers.google.com/workspace/gmail/api/auth/scopes).
- **Scheduled send (critical).** Compose now and send at a chosen time. Not implemented. Initial personal setup: a durable local queue and small helper on the user's always-on Mac Studio. Gmail's documented API has immediate send methods, not a scheduled-delivery endpoint, so the helper must be running with network access at the due time and call Gmail to send then. Recover missed sends after interruption. This avoids requiring a Postilaatikko hosted sending service for this setup.
- **Unified virtual folders.** Saved sets of real folders across accounts, shown in the sidebar and filtered to unread mail for a lightweight to-do list. Implemented in Postilaatikko v0.3.10.
- **Cross-account message moves.** Already supported. Preserve the safety rule: confirm the destination copy before removing the source.
- **Multiple sender identities across accounts.** Already supported in the composer. Improve it with contextual signature selection (sender, recipient, and message type); signatures can already be configured per identity.
- **Local cross-account filters.** Not started. Build on the existing cross-account move action; match incoming messages and route them to a folder in another account.
- **Gmail-style keyboard bindings.** Fast list navigation and common mail actions, safe while typing and documented in `docs/KEYBOARD_SHORTCUTS.md`. Not started.
- **Gmail-native spam actions.** Let users mark messages as spam and undo that action for Gmail accounts, invoking Gmail's native spam classification/disposition rather than only moving mail to a local folder. The Gmail API supports applying/removing the `SPAM` system label; confirm the exact classifier-training behavior during implementation. Not started.
- **Reply and email templates.** Save reusable short replies and complete message templates. Not started; use the existing identity and signature setup.
- **Calendar recognition.** Turn recognized date/time mentions in message content into links that can open or create events in installed calendar applications. Not started; build on `extensions/calendar`.
- **Multiple mail windows.** Keep more than one mail window open for parallel reading and searching. Not started; the detached compose window already exists.

## Planned

- **Smart notifications.** Per-sender and rule-based controls for deciding which messages produce notifications. Not started; lower priority.
- **Snooze and reminders.** Hide or remind about a conversation at a chosen time. Not started and lower priority than the core items above.

## Open questions

- **Scheduled send from other devices / for other users.** The initial personal setup uses a local Mac Studio helper. Decide later how other Postilaatikko clients securely deliver queued messages to that helper, and whether a hosted service is needed for users without an always-on machine. Gmail's public API does not expose a scheduled-send endpoint; sends happen immediately when the helper calls the API.
- **Gmail-style key map.** Choose which Gmail bindings to support and make sure they do not fire while typing in inputs.
- **Calendar link format.** `webcal:`/ICS download, a Google Calendar template URL, or the OS default calendar.

## Parked

- Nothing parked right now.

## Done

- **Unified virtual folders (v0.3.10).** Saved unread views across selected folders and accounts, including sidebar management and search.
- **Postilaatikko product naming (v0.3.10).** Updated visible product branding while retaining compatibility-sensitive app identifiers.
