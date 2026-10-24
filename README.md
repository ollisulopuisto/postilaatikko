# Postilaatikko

**A personal, open-source desktop email client for people who use multiple accounts.**

Postilaatikko is this fork's product name. The project builds on the work of Eterno Mail and Aerion, with changes intended to be offered back to Eterno Mail as a pull request once they are ready.

<p align="center">
  <strong>Fork lineage</strong><br>
  <a href="https://github.com/hkdb/aerion">Aerion</a> by <a href="https://3df.io">hkdb / 3DF</a>
  &nbsp;→&nbsp;
  <a href="https://github.com/wesleiaqui/EternoMail">Eterno Mail</a> (immediate upstream)
  &nbsp;→&nbsp;
  <a href="https://github.com/ollisulopuisto/postilaatikko">Postilaatikko</a> (this fork)
</p>

<p align="center">
  <img src="docs/ss.png" alt="Postilaatikko mail interface" width="900">
</p>

## Why this fork exists

The aim is a capable personal mail client for working across several of your own accounts. The product direction takes useful interaction ideas from Spark and Gmail and focuses on fast everyday mail, clear navigation, and cross-account organization.

Scheduled send is a critical priority for this fork. Other priorities include Gmail-style keyboard bindings, reply and email templates, contextual signatures, local cross-account filters, calendar links recognized from email, and multiple mail windows. Cross-account sender identities and message moves already exist. Sender-aware notifications are on the roadmap at a lower priority. See [ROADMAP.md](ROADMAP.md) for status and scope.

This is for personal mail workflows. Shared inboxes, team assignment, and collaboration features are out of scope.

## Relationship to upstream

This repository's Git remotes identify its direct lineage:

- **This fork (`origin`):** [ollisulopuisto/postilaatikko](https://github.com/ollisulopuisto/postilaatikko)
- **Immediate upstream (`upstream`):** [wesleiaqui/EternoMail](https://github.com/wesleiaqui/EternoMail)
- **Earlier upstream:** [hkdb/aerion](https://github.com/hkdb/aerion), the project Eterno Mail forked from

Postilaatikko keeps compatibility-sensitive Go module and desktop application identifiers from its upstreams for now. The visible product name is Postilaatikko. Changes are being developed with the intention of proposing a pull request to Eterno Mail after they are stable; the lineage above credits both upstream projects and their authors.

## What works today

- Multiple email accounts with IMAP and SMTP, including provider OAuth flows.
- Unified inbox and aggregate special-folder views across enabled accounts.
- Message search, conversation views, attachments, drafts, and common mail actions.
- Contacts and calendar extensions, with local data and account integrations.
- Linux, macOS, and Windows desktop builds through Wails.

The fork is actively changing. Check the roadmap before relying on a feature described as planned or in progress. **Scheduled send (“send later”) is not implemented.**

## Build and run from source

The application uses Go, Wails, Svelte, TypeScript, and SQLite. You need Go 1.25 or newer, Node.js with npm, Wails' platform build dependencies, and the platform libraries required by WebKitGTK when building on Linux. See the [Wails installation guide](https://wails.io/docs/gettingstarted/installation) for operating-system prerequisites.

```sh
git clone https://github.com/ollisulopuisto/postilaatikko.git
cd postilaatikko
make dev
```

Build a production binary with:

```sh
make build
```

Some OAuth integrations need provider client configuration for development; see `.env.example`. Never commit credentials or local `.env` files.

## Contributing

The short-term goal is to make focused, reviewable improvements and offer suitable changes to [Eterno Mail](https://github.com/wesleiaqui/EternoMail) as pull requests. Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [ROADMAP.md](ROADMAP.md) first. Changes that affect account safety must preserve mail: when moving between accounts, copy successfully before deleting the source.

## License

Postilaatikko is distributed under the [Apache License 2.0](LICENSE), following its upstream project.
