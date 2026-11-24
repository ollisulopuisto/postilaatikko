# Headless account setup

`postilaatikko-helper accounts add` configures the helper's own local profile without a graphical app. This is local administration, not a remote account-creation endpoint. Stop both Postilaatikko and its helper first. Setup takes an exclusive profile/schema lock and refuses an active profile. Back up the profile before upgrading. It can create a new profile and applies the existing database migrations.

## Google or Microsoft

On the Ubuntu host:

```sh
postilaatikko-helper accounts add --provider google --email you@example.com
# or --provider microsoft
```

Keep the command running. It prints a browser consent URL and the random loopback port. If your browser is on your Mac, first open a second terminal there and forward **that exact port** to the Ubuntu host:

```sh
ssh -N -L PORT:127.0.0.1:PORT USER@HOST
```

Replace PORT, USER and HOST, then open the printed consent URL on the Mac. Keep the tunnel open until setup finishes. On the same computer, no SSH tunnel is needed. The callback listens only on loopback. The five-minute session uses the existing PKCE/state checks; cancellation, wrong identity and missing offline refresh grants do not create an account. `--email` is optional for OAuth, but when supplied it must match the signed-in identity. No callback URL, authorization code or token should be pasted into chat or a shell command.

The existing shipped desktop client configuration is used, with this profile's saved client override/selection and existing environment defaults. This command does not add a new OAuth project, widen scopes, enable APIs or bypass Google's consent requirements. Google may require a verified/test-user-approved OAuth project. It uses desktop loopback authorization, not Google's limited-device or retired out-of-band flow.

OAuth Gmail accounts use the existing Gmail API transport, although their legacy account configuration still contains `imap.gmail.com`/`smtp.gmail.com`. Microsoft mail uses the existing IMAP/SMTP OAuth transport. No new transport is introduced here. The command adds the account only, does not test a real mailbox, sync or send. Restart the helper afterward.

## Generic IMAP

```sh
postilaatikko-helper accounts add --provider imap --email you@example.com \
  --imap-host imap.example.com --smtp-host smtp.example.com
```

Passwords are read directly from a terminal with echo disabled. There are no password/token command-line flags, environment password option, pipe or secret-file input. Use an app password when the provider requires one. Incoming TLS defaults to port993, outgoing STARTTLS to587; override ports with `--imap-port`/`--smtp-port`. Unencrypted authentication is not offered. `--username` overrides the incoming login, `--smtp-username` prompts for a separate outgoing password, and `--receive-only` disables outgoing mail. `--name`, `--display-name` and `--data` are optional. Provider reachability/password validity are checked later by normal sync, not by this setup command.

## Storage and limits

Mail secrets use the existing credential store: OS keyring when available, otherwise encrypted SQLite data and a private `device.key`. This fallback is not equivalent to a hardware-backed keychain. Its key derivation uses host/UID plus local key material, so copying a profile is not a supported account transfer. Protect the entire profile, backups and OS account; no export/import feature is added. Setup output contains the account ID/email and consent URL, never passwords or tokens. Provider/backend logs are disabled for this setup command to prevent error-body disclosure. Do not redirect the consent session into shared logs.

A failed credential write removes new credentials and the new account. If cleanup fails, a disabled record is retained for repair instead of claiming success. Existing accounts are not replaced. This command neither edits existing provider grants nor migrates Gmail consent scopes.

Send Later's pairing identity is separate and currently still needs an unlocked OS keyring. This account-setup change does not make `init`/pairing headless. A separate reviewed change is required for that identity storage. Real Google/Microsoft consent, Ubuntu desktop Secret Service behavior and real mailbox sync/send remain acceptance checks; tests here use isolated local fixtures.

Setup may create/migrate an empty profile before a password prompt is cancelled; it does not create an account until credentials are available. Password strings cannot be reliably erased from Go memory; input byte buffers are cleared, but this is not a secure-memory guarantee. If setup selects the OS keyring, the service must later run under the same user with that keyring available. Starting later without it cannot recover keyring-only secrets through the encrypted fallback. For a keyring-less service, run setup in that same keyring-less user/session. OAuth setup requires the email from an authenticated provider userinfo request, not an unsigned ID-token claim, and rejects an explicitly partial Gmail mail grant.
