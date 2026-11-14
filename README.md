# Postilaatikko

Postilaatikko is a desktop email client for managing several of your own accounts. It combines IMAP/SMTP mail and Gmail API features with unified folders, conversation reading, and personal triage queues. The desktop uses Go, Wails, Svelte and SQLite.

This is an actively developed fork of [Eterno Mail](https://github.com/wesleiaqui/EternoMail), which is based on [Aerion](https://github.com/hkdb/aerion). It is a personal mail workspace, not a shared-inbox or team-assignment service.

## Install

Get an available package from this repository's [Releases](https://github.com/ollisulopuisto/postilaatikko/releases). Check the release's version, source commit, platform and architecture before installing. Test builds are not stable releases; private draft releases require repository access.

For a macOS DMG:

1. Download the DMG. If that release includes `SHA256.txt`, save both files in the same directory and run `shasum -a 256 -c SHA256.txt` there. Older tagged releases may omit this file; compare `shasum -a 256 path/to/download.dmg` with the asset digest when available instead.
2. Open the DMG and drag **Postilaatikko.app** to **Applications**.
3. Open the app and add your mail accounts. Provider authorization and optional contacts/calendar access are separate setup steps.

The macOS release workflow signs and notarizes its app and DMG. A local development build is ad-hoc signed and is not notarized. Do not disable system-wide security protections to run an unverified download.

The source also has Linux, Windows and Flatpak build paths. Availability of a package does not establish that every provider or native integration has been tested on that platform. See [Build](docs/BUILD.md) and [Release](docs/RELEASE.md) for packaging details.

## Mail and organization

- **Several accounts in one workspace.** Browse individual accounts or All Inboxes, plus unified Sent, Drafts, Trash, Spam, Starred and Archive views. Configure sender identities and signatures.
- **Two ways to read.** Switch between Conversation and Traditional readers, keep messages in reader tabs, and use inline or detached compose windows. Duplicate label copies are collapsed in thread reads.
- **Personal queues.** Read Later, Reply Later and Snooze sit alongside saved virtual folders. Virtual-folder criteria can span folders and accounts. Reply Later is a reminder queue, not a scheduled send. Send Later can repeat and has a simple mailing-list mode (you in To, everyone else in Cc or Bcc, one identical message).
- **Search and filters.** Search cached mail, use Gmail's server-side search where authorized, and apply local cross-account filter rules. Moves confirm the destination copy before removing the source.
- **Everyday compose.** Drafts, attachments, reply/forward, identities, signatures and saved quick-reply snippets. Keyboard shortcuts share a searchable reference and command palette.
- **Contacts and calendar tools.** Account-backed contact sources and calendar extensions; macOS Contacts is an optional, permission-gated read source. Native contact edits happen in Apple's Contacts app.

Routine sync is header-first. Bodies and attachments are fetched on demand, with bounded body-prefetch options for selected folders. A search or thread with incomplete local content is not proof that the provider has no more mail.

These are implemented code paths, not a promise of full provider parity or native acceptance. See the [changelog](CHANGELOG.md), [keyboard reference](docs/KEYBOARD_SHORTCUTS.md) and [macOS Contacts notes](docs/MACOS_CONTACTS.md) for details and remaining checks. [ROADMAP.md](ROADMAP.md) and [TASKS.md](TASKS.md) include plans and historical status, which can lag the code.

## Background service and Send Later

The bundled `postilaatikko-helper` can own mailbox sync and automatic filters independently of the desktop. On macOS, General Settings exposes installation, start and stop controls for the background service. The helper must remain running, connected and able to access its credentials; closing the desktop is not the same as shutting down its host.

**Send Later is implemented but still experimental.** The composer and Scheduled view use a paired helper with a durable encrypted queue. Setup requires verifying device/helper keys and mapping sender permissions. Live delivery, keychain/login lifecycle, failure recovery and independent security review remain release gates. Do not rely on it for time-critical mail without testing your own setup. See [Send Later](docs/SEND_LATER.md), including cancellation and uncertain-delivery handling.

The helper also exposes a capability-checked mailbox API. A desktop can use a user-operated remote helper over HTTPS with a separate revocable device credential. The service host holds account credentials and mailbox data: it is a trusted endpoint, not a service blind to your mail. Remote errors fail closed rather than silently switching to a different mailbox. See [remote service setup](docs/REMOTE_MAILBOX_SERVICE.md) and the [API guide](docs/API.md).

## Build from source

Use Go **1.26+**, Node.js **24** with npm, and Wails **v2.15.0**. Install the operating-system dependencies from the [Wails installation guide](https://wails.io/docs/gettingstarted/installation). Linux builds use the `webkit2_41` tag and need GTK/WebKitGTK development libraries.

```sh
git clone https://github.com/ollisulopuisto/postilaatikko.git
cd postilaatikko
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
# Ensure the Go bin directory is on PATH.
npm ci --prefix frontend
make dev
```

`make build` produces the desktop app and helper. On macOS, the repository's local script installs the pinned Wails CLI, checks prerequisites, builds the app and creates a drag-and-drop DMG:

```sh
./scripts/build-macos-local.sh
```

Its outputs go to `build/bin/`; this script does not publish a release or notarize the app. Run it with `--help` for app-only and repeat-build options.

Provider OAuth development overrides are described in `.env.example`. Configure your own provider registration when needed; do not commit credentials or local `.env` files. Compatibility-sensitive names such as the Go module `github.com/hkdb/aerion` and executable `eterno-mail` remain in the source.

For checks:

```sh
make test
npm test --prefix frontend
npm run check --prefix frontend
npm run lint --prefix frontend
```

Native Go checks require the platform libraries, not just the frontend dependencies. Passing automated checks does not replace testing the installed app against real accounts. See [CONTRIBUTING.md](CONTRIBUTING.md) for conventions and [TASKS.md](TASKS.md) for current work.

## Privacy and safety

Mail caches, drafts and settings are stored in local databases; account passwords and tokens use the operating system's credential storage. Protect the device and its backups. Removing the app may leave its data behind.

Using an email client still involves your configured providers. Loading remote message images can contact their hosts; image-loading preferences and sender allowlists control that behavior. Optional account integrations need their own permissions.

A remote helper changes where mailbox data and sending credentials live. Protect that host, its database, logs, backups, TLS keys and per-device grants. Send Later's paired-device encryption does not hide mail from the helper that must decrypt it to send.

Never attach real mail, account tokens or unredacted logs to a public bug report. See [SECURITY.md](SECURITY.md) for the inherited vulnerability-reporting policy; its upstream contact details are not a promise of support for this fork.

## License and upstream credit

[Apache License 2.0](LICENSE). Postilaatikko retains the work of Eterno Mail and Aerion and aims to offer suitable changes back upstream. This fork's name does not erase their authorship or the identifiers retained for compatibility.
