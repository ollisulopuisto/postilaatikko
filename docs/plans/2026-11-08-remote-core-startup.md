# Remote core workspace startup

**Goal:** Boot the desktop against a configured remote helper without opening the local mailbox database, and let the core mail workspace browse and read helper-owned mail.

## Scope

- Add a remote-mode preflight branch after device DB schema initialization and endpoint lookup. Local mode keeps the current helper bootstrap, schema lock, mailbox DB, and copy-only legacy import sequence.
- Add a remote-specific `App.Startup` path that initializes device state and Wails readiness without constructing service stores, database-backed undo, provider pools, filters, local sync, draft-sync, or FTS workers.
- Route account/folder/view catalog, helper-owned virtual-folder cached views/counts, bounded folder/unified-inbox conversation pages and thread reads through the authenticated remote client. No local service fallback is allowed in remote mode.
- Add a `mail.read` conversation-count operation where needed by the desktop so counts do not scan all pages or masquerade as cached-page totals.
- Guard optional/local-only entry points so missing stores produce an explicit unsupported-mode error rather than a nil dereference or empty result.

## Acceptance

- With a remote endpoint and keyring grant, `Preflight` succeeds using only `device.db`; it does not create the mailbox DB, run helper bootstrap, acquire the schema lock, or initialize OAuth credential stores.
- With no remote endpoint, existing local-helper preflight remains compatible and performs deferred legacy imports.
- `Startup` emits `app:ready` for remote mode with the minimum device-owned stores and does not start local sync/provider/background index work.
- The remote workspace loads account/folder/view navigation, an Inbox page, a saved-view cached page/count, and a conversation thread from an HTTPS fixture without a mailbox DB. Body fetch remains on demand.
- Remote authentication, capability, and network errors propagate as errors; they never select a local fallback or render as a successful empty mailbox.
- Local startup and local-helper unavailable behavior remain unchanged.
- Focused tests, affected package builds, and `git diff --check` pass.

## Known boundaries

Compose/send, attachment-byte/inline attachment reads, raw source, unified views other than Inbox, per-account synthetic Archive, physical-folder actions outside existing service routes, triage queues, contacts/CardDAV, crypto integrations, presentation-cache persistence, and targeted server change events remain follow-up slices. These methods return explicit unsupported errors in remote mode where invoked; no empty shadow stores are initialized.

## Status

The remote preflight and core read path are implemented and verified. The desktop can open `device.db`, read its configured HTTPS endpoint, and reach Wails readiness without creating the mailbox DB. Core account/folder/saved-view navigation, helper-cached views/counts, bounded folder conversation pages/counts, thread reads, and explicit body fetch use the helper. Unsupported remote scopes return errors. See HOSTED-SERVER-051 in `TASKS.md` for the commit and verification record.
