# Roadmap

Planned, in flight, open questions, and parked items for this fork. Done items are marked with the version that shipped them.

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

- **Virtual folders.** A saved set of folders, possibly across accounts, filtered to unread, listed in the sidebar like a real folder. Unread mail is used as a to-do list.
  - Storage (decided): a `virtual_folders` table plus a membership table referencing `folders(id)` with `ON DELETE CASCADE`, so removing a folder or account cleans up its references. One migration in `internal/database/migrations.go`.
  - Reuse: `ListConversationsByFolderIDs` and `CountConversationsByFolderIDs` in `internal/message/store.go` already take any folder ID list with an `unread` filter; the unified inbox (`app/unified.go`) uses them across accounts.
  - Not started.
- **Scheduled send.** Compose now, send at a chosen time. Not started. Delivery depends on the open question below.

## Planned

- **Snooze.** Hide a conversation until a chosen time. Not started. Depends on the delivery question below.
- **Reminders.** Remind about a conversation at a chosen time. Not started. Depends on the delivery question below.
- **Calendar recognition.** Detect date/time mentions in message bodies and offer each as a link that opens the calendar app. A calendar extension already exists under `extensions/calendar`; check it before adding a second path.
- **Multiple inboxes.** User-defined inboxes: named sets of folders, from one or more accounts. The unified inbox is the existing pattern. Not started.
  - Moving within one account is an IMAP folder move.
  - Moving across accounts is copy, then delete on the source. A failed copy must never lose mail, so delete only after the copy is confirmed.
- **In-app filter rules.** Match incoming or existing mail (sender, subject, recipients and similar) and move it to a folder or inbox, including across accounts. Gmail filters can't move mail into another account, so this is needed. Depends on multiple inboxes. Not started.
- **Keyboard shortcuts.** Extend the existing scheme in `docs/KEYBOARD_SHORTCUTS.md` so each new feature has a binding, and keep that doc in sync. Not started.

## Open questions

- **Delivery while the app is closed.** Scheduled send, snooze and reminders need a trigger. Options: (A) fire only while the app is open and catch up overdue items on launch, or (B) a background process that survives the window closing and starts at login. Gmail's API has no server-side schedule for these sends, so the choice is local. Leaning to A first.
- **Single-key navigation.** Add Gmail-style j/k/e/# keys? Needs care so keys don't fire while typing in inputs.
- **Calendar link format.** `webcal:`/ICS download, a Google Calendar template URL, or the OS default calendar.

## Parked

- Nothing parked right now.

## Done

- Nothing new in this fork yet beyond the upstream Eterno Mail v0.3.x features in `CHANGELOG.md`.
