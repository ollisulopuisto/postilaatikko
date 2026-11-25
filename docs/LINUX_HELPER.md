# Linux helper user service

This guide describes the Linux systemd user-service implementation. It is not a claim that deployment, logout, reboot or real-mail delivery has been tested on a clean Ubuntu VM.

## Requirements and scope

Use a Linux build of `postilaatikko-helper`, systemd with a working user manager, and an already configured helper-local account profile. Keep the executable at a stable absolute path. Do not run the user service as root. The helper does not install itself or create mail accounts through `install-agent`.

The built-in installer currently supports **keyring identities only**. The user's Secret Service must be available and unlocked when the service starts. Enabling linger does not unlock a keyring after reboot. Headless boot operation is therefore not promised by this installer.

The explicit `--identity-store encrypted-file` backend exists for manual helper commands, but `install-agent` rejects it before changing anything. Service flag forwarding is a separate pending change. For that backend's setup, limitations and threat boundary, read [HEADLESS_IDENTITY.md](HEADLESS_IDENTITY.md). Do not omit the backend flag or assume a same-named keyring identity is the same identity.

## Prepare the profile

Replace the paths below with absolute private paths. Use the same identity, data and queue options for all setup and administration commands. Never point `--data` at another machine's copied live database.

```sh
HELPER=/absolute/path/postilaatikko-helper
DATA=/absolute/private/mail-profile
QUEUE=/absolute/private/helper-queue
"$HELPER" bootstrap --data "$DATA"
```

`bootstrap` initializes or upgrades the helper database. When an installed helper is running, the Linux migration-pause path stops it before upgrading and the existing bootstrap logic restarts it afterward. Inspection or stop failures abort the upgrade. Do not run competing helpers against the same profile.

Configure accounts using the [headless account guide](HEADLESS_ACCOUNTS.md). If creating a new keyring identity is needed, initialize it once:

```sh
"$HELPER" init --identity studio --data "$DATA"
"$HELPER" public-key --identity studio --data "$DATA"
```

Do not reinitialize an existing identity. Verify public keys independently before pairing; see [SEND_LATER.md](SEND_LATER.md). No new pairing handshake is included here.

## Install, inspect and control

The following installs a loopback-only Send Later service without mailbox sync:

```sh
"$HELPER" install-agent --identity studio --data "$DATA" --queue "$QUEUE" --no-sync
"$HELPER" agent-status
"$HELPER" stop-agent
"$HELPER" start-agent
```

Omitting `--no-sync` from `install-agent` enables mailbox sync, so choose deliberately. Existing TLS, host-allowlist and API listener flags can be passed at install time. Do not expose either listener remotely without following the TLS and host-allowlist requirements in SEND_LATER.md.

The installer exclusively creates `~/.config/systemd/user/postilaatikko-helper.service` with mode 0600 and newly created directories with mode 0700. It refuses to overwrite an existing unit, including a symlink. It runs `systemctl --user daemon-reload` and `systemctl --user enable --now postilaatikko-helper.service`. If either command fails, it reports the error and leaves the saved unit for inspection. Stop/uninstall it before changing its configuration.

`agent-status` reports `supported`, `installed` and `running`. A missing user bus is an error, not proof the service is inactive. Inspect the unit and logs directly:

```sh
systemctl --user status postilaatikko-helper.service
journalctl --user -u postilaatikko-helper.service
curl --fail http://127.0.0.1:8028/healthz
```

Use the configured port if it differs. Health checks only prove the Send Later process responds, not that accounts are paired, mailbox sync works or messages were delivered. The authenticated mailbox API is separate.

## Logout and boot

The unit uses `Restart=on-failure` and `WantedBy=default.target`. A user manager normally follows login. To ask systemd to keep the user manager after logout and start it at boot, an operator can run:

```sh
loginctl enable-linger "$USER"
loginctl show-user "$USER" -p Linger
```

This may require administrator permission. It does not keep an offline/sleeping computer sending, unlock a keyring, guarantee network availability, or prove successful service startup. Verify logs and health after logout and a reboot before relying on it. Encrypted-file service installation is not supported by this slice.

## Uninstall

```sh
"$HELPER" uninstall-agent
```

This disables and stops the user unit before removing its file, then reloads systemd. Stop/disable failure preserves the unit. It does not delete accounts, queues, identities or mail data. Linger is independent; if you enabled it only for this helper, review other user services before considering `loginctl disable-linger "$USER"`.

## Manual VM smoke

Read `scripts/linux-helper-smoke.sh --help` before running it in a disposable Ubuntu LTS VM. It requires an existing throwaway keyring identity and an unlocked Secret Service, refuses an existing service unit or profile, bootstraps a new isolated database, installs without sync, restarts, probes health and uninstalls. It retains the profile for inspection. It is not wired into CI and does not initialize/delete identities, enable linger, reboot, pair devices or send mail. It cannot validate a headless encrypted-file deployment while service forwarding is unsupported.
