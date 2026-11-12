# Connect a desktop to a remote mailbox service

The desktop can connect to a user-operated Postilaatikko helper on another computer. The service host owns account credentials and mailbox data. Remote access uses the same authenticated API as localhost, with HTTPS and per-device revocable API grants.

## Prepare the helper

Configure a trusted TLS certificate whose names match the service URL, then start the helper's API listener on the intended network interface. Set the exact accepted host name with `--api-allowed-host`; non-loopback API binds require both TLS files and an allowlist. For example:

```sh
postilaatikko-helper serve \
  --api-listen 0.0.0.0:8029 \
  --api-tls-cert /path/to/fullchain.pem \
  --api-tls-key /path/to/private-key.pem \
  --api-allowed-host mail.example.net \
  --sync
```

Expose the port only through the operator's intended firewall or reverse-proxy configuration. The helper's Host allowlist must contain the exact host in the desktop URL. Do not expose the API over plain HTTP.

Create a separate, least-privilege API grant for each desktop. The minimum for browsing and reading is:

```sh
postilaatikko-helper api-client create \
  --name "Laptop desktop" \
  --capability mail.read \
  --capability mail.body.fetch
```

The command prints a one-time bearer token and a client ID. Transfer the token to that desktop through a private channel. It is stored in the desktop's OS keychain; do not put it in shell history, source control, logs, or a URL. Add only capabilities the device needs when creating the grant. The current helper CLI creates a replacement grant to change capabilities.

## Connect and revoke

In Postilaatikko, open **Settings → General → Mailbox service**, enter the helper's HTTPS URL (for example `https://mail.example.net:8029`) and the one-time token, then connect. The desktop checks the helper's API version and required read capabilities before saving the connection. It uses the platform's normal TLS certificate and hostname validation.

Disconnecting removes the local keychain token and restores the default localhost endpoint. It does not revoke the grant on the helper. To revoke it, use the client ID from creation:

```sh
postilaatikko-helper api-client revoke --client CLIENT_ID
```

List active clients with `postilaatikko-helper api-client list`. Revocation takes effect on subsequent API requests. If a configured remote service is unavailable or rejects the device, the desktop reports the failure and does not switch to its local mailbox database.

The helper host is trusted with account authorization and mailbox data, including prepared content needed for scheduled sending. This connection is not end-to-end encrypted from the service operator. Protect the host, its credentials, database, logs, backups, TLS keys, and network access accordingly.

## What runs on each computer

When a remote endpoint is already configured, desktop startup opens only the device database. It does not open the local mailbox database, construct local provider clients, or start local mailbox sync workers. The helper owns mailbox storage and provider work; the desktop keeps rendering, device preferences, content policy and certificate trust locally. Changing an existing desktop connection does not retroactively prove that its current process has released local resources: restart the desktop to enter the remote-only startup path.

Supported remote paths include mailbox lists/search/counts, message and attachment reads, drafts and immediate send through the helper, and mailbox actions with helper-owned Undo. A configured remote service failure is reported without silently switching to the local mailbox database.

Remote mode is not yet at local-mode feature parity. Contacts/CardDAV, Calendar service operations and S/MIME/PGP remain explicitly unavailable. The existing Undo Send queue uses the local mailbox database and its remote dispatch is blocked, so do not rely on it remotely. Native two-computer delivery, failure recovery and CPU/RSS savings remain acceptance work, not guarantees established by compilation or API tests.

The first connection still needs a way to configure the helper URL and grant; configuring it is distinct from migrating accounts, provider credentials or mail to a new host. Back up and provision the trusted helper separately. Connecting a desktop must not be treated as implicit permission to transfer private signing/decryption keys.

For source details, see `app/app.go` (`Preflight`, `startRemoteWorkspace`) and the HOSTED-SERVER-051 onward entries in `TASKS.md`. The owner-operated mailbox mode described here is distinct from the hosted zero-knowledge design, which must not keep a full mailbox or provider credentials on its server.
