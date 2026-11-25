# Explicit headless Send Later identity storage (Linux)

Design choice: keep OS keyring as the default. An operator can explicitly select `--identity-store encrypted-file` for a Linux helper that has no Secret Service/keyring. This is not an automatic fallback, not a migration and not a claim of hardware-backed key protection. The desktop client still uses its existing keyring identity. Pairing permissions and message encryption are unchanged.

Initialize a new, separate helper identity on Linux:

```sh
postilaatikko-helper init --identity-store encrypted-file --data /absolute/private/profile --identity studio
postilaatikko-helper public-key --identity-store encrypted-file --data /absolute/private/profile --identity studio
```

Use the same three options with `pair`, `serve`, `revoke`, `list` and other commands that load this identity. Verify public keys directly between devices before pairing, as before. Do not send private keys, passwords or tokens through chat. Only the public key is printed. Do not initialize a new backend under the assumption it will preserve an existing keyring pairing: it creates a different identity, and the remote device must be paired deliberately.

Linux `install-agent` supports keyring identities only; it explicitly rejects `--identity-store encrypted-file`. See [Linux helper services](LINUX_HELPER.md). For an encrypted-file identity, use your existing service manager. Its explicit command must retain `--identity-store encrypted-file`, the absolute `--data` directory and `--identity` name. Omitting the option selects keyring and fails if that keyring identity is unavailable; it does not silently load the file. macOS login-agent installation and the desktop identity path remain keyring-only and unchanged. The encrypted-file backend itself adds no system service or startup behavior.

## Storage and failure behavior

The selected profile and `send-later-identities` directory must be real private directories (0700). Identity and device-key files must be private regular files (0600). Names are hashed for filenames, and the encrypted payload binds the original name. Symlink identity/key files, shared permissions, corruption, missing encryption keys and mismatched names fail closed. An existing identity, even corrupt, is never replaced by `init`. A process lock and atomic no-replace file publication serialize concurrent initializers. No load failure generates a replacement identity.

The NaCl private key is encrypted using the application's existing AES-GCM device encryptor in this separate identity directory. This encryptor derives its key from local host/UID information and stored key material. It is software obfuscation/at-rest protection under the local OS account boundary, not protection against an attacker who can read the complete profile or compromise the host. Directory/file access controls and encrypted disks/backups remain important. The key file and identity must be backed up together, but restoring on a different hostname or UID is not supported by this scheme. This is deliberately documented rather than presented as a secure cross-host export feature.

If a process dies before publication, initialization may leave an unused private temporary encrypted file; it is not treated as an identity. If publication succeeds but the final directory sync reports failure, initialization reports an error and the next attempt refuses the existing identity. Use `public-key` to inspect the resulting public identity, never blindly initialize or delete keys. There is no automatic repair, deletion or rotation command in this change.

## Verification limits

Tests use isolated profiles: restart stability, absent keyring, duplicate initialization, concurrent initialization, corruption, missing key, file permissions, symlinks and name binding. A Linux helper process can initialize and show its public key without D-Bus. Real Ubuntu service-manager deployment, macOS keychain pairing and delivery to a real mailbox are not claimed. Account creation is a separate PR; this PR does not add accounts or change OAuth.

The backend flag is not persisted: a service unit without `--identity-store encrypted-file` may load a different, same-named keyring identity if one exists. Always pin backend/data/name in its command and verify the public key after service changes. A hostname or UID change, including container recreation under another identity, makes the encrypted identity undecryptable and fails closed. Do not treat container rebuilds as transparent key migrations.
