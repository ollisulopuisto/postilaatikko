# Postilaatikko identity and compatibility

## Changed

- Desktop executable: `postilaatikko` (`postilaatikko.exe` on Windows).
- Go module/imports/linker symbols: `github.com/ollisulopuisto/postilaatikko`.
- Linux desktop, Flatpak, Windows notification and macOS login-agent identity:
  `io.github.ollisulopuisto.postilaatikko`.
- Windows login registry value: `Postilaatikko`.
- Packaging assets, manifests and executable paths follow those names.
- New diagnostic environment names: `POSTILAATIKKO_DEBUG`,
  `POSTILAATIKKO_PPROF`, `POSTILAATIKKO_API_TIMING`.
  The corresponding `AERION_DEBUG`, `ETERNOMAIL_PPROF` and
  `ETERNOMAIL_API_TIMING` switches remain accepted compatibility aliases.

## Intentionally unchanged

Existing native user-data/cache paths, `aerion.db`, credential/keychain service
names, OAuth selections, contact source IDs, crypto derivation inputs and saved
browser-storage keys stay unchanged. These are data contracts, not branding.
Changing them blindly would make existing accounts, encrypted values or UI
preferences disappear. Native app data is not copied or deleted by this rename.
The macOS bundle identifier stays `com.wails.postilaatikko` to preserve its OS
identity. Historical release notes and upstream attribution remain historical.

The Linux installer provides an `eterno-mail` command symlink for older launch
scripts. The macOS and Windows executable names really change; custom scripts
that point into the app bundle or at the old `.exe` must be updated. Existing
native login entries are migrated to the new executable on application startup,
only when a legacy entry already exists. Enable/Disable removes old entries to
avoid duplicate login launches. A currently loaded macOS login job can remain
loaded until logout; quit the old app before installing and verify login startup
after the next login. The app does not run launchctl or kill an old process.

## Existing Flatpak installations

The new ID is a different Flatpak application. Merely installing a new bundle
will not carry the old sandbox's files or permissions. The new sandbox cannot
read the old `~/.var/app/io.github.wesleiaqui.eternomail` by default, and this PR
does not add broad home-directory access to get around that protection.

A distributor controlling the old remote can publish an end-of-life rebase to
the new ID. Flatpak can then offer the replacement and preserve the application
data through its previous-ID mechanism. This requires publication by the old
remote's maintainer; changing the manifests here does not update an upstream
Flathub entry or an already-installed release. No remote is published by this PR.
The `X-Flatpak-RenamedFrom` desktop metadata and AppStream replacement metadata
record the relationship but do not replace that distributor operation.

For separate bundle installs, on the host, before starting the new app:

1. Quit both old and new applications, including any background instance.
2. Keep a backup of the old profile and credentials. Do not uninstall with
   `--delete-data`.
3. Run `python3 scripts/migrate-flatpak-data.py` from this checkout. It verifies
   neither ID is listed by `flatpak ps`, refuses an existing new profile and
   symlinked old profile, copies the old profile, and leaves the original intact.
   If the copy fails, inspect the retained partial destination before retrying.
   Do not run either application during the copy. This is a host operation, not
   an operation the new sandbox can perform automatically.
4. Install/run the new bundle as `io.github.ollisulopuisto.postilaatikko`.
   Verify accounts, cached mail, drafts, pending sends and preferences before
   removing the old installation. Do not run both profiles concurrently because
   copied pending work could otherwise execute twice.
5. Disable old-ID background/autostart permissions from the desktop's app
   permissions UI, and enable startup in the new app if wanted. Review any
   custom old-ID `flatpak override` settings rather than copying permissions
   blindly. Portal grants, desktop defaults, pinned launchers and secret-service
   access prompts may need confirmation again. Credentials retain their service
   names, but this is not a guarantee that a desktop's keyring policy grants the
   new application access automatically.

The shipped `.flatpakrepo` still points at the legacy distribution endpoint.
Deployment operators must verify or replace it with the actual published remote
before distributing it as an installation entry point. This rename does not
claim control of that host or make an upstream repository current.

## Verification limits

Automated tests cover module compilation, old/new debug switches, non-overwriting
profile copying, autostart migration and retained unrelated login entries. Full
native macOS/Windows installations and an actual Flatpak upgrade are separate
platform acceptance checks. Do not call this an exercised live migration.

Flatpak references:
- https://docs.flatpak.org/en/latest/flatpak-command-reference.html
- https://github.com/flatpak/flatpak/pull/2775
