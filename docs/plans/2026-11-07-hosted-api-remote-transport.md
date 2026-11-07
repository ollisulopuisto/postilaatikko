# Hosted API remote transport

This slice adds a secure transport mode to `internal/automation.Server` for the user-operated Mac Studio/VPS deployment described in the always-on service plan. It does not start a listener from the helper or make remote access available by default.

## Configuration contract

- An empty address continues to use an ephemeral loopback HTTP listener.
- A loopback listener may use HTTP, preserving current local development and client behavior.
- A non-loopback listener requires a TLS certificate, matching private key, and one or more exact allowed Host names. Invalid certificate pairs fail before the listener starts.
- The same bearer-token validation, revocation, capability checks, request limits, and browser-Origin rejection apply in both modes.
- Host headers are normalized for case, a trailing dot, and optional port before exact allowlist matching. Wildcards are not accepted.
- `URL()` reports `https://` when TLS is configured.

The helper still needs a separate integration task to open the durable grant store, mount the headless catalog handler, expose operator flags, and manage listener shutdown with the sync and scheduled-send workers. Remote access must not be enabled until those pieces and deployment documentation are reviewed together.

## Follow-up

Wire this transport into the independently supervised helper after durable client grants and catalog reads are integrated on `main`. Document certificate provisioning, DNS, firewall rules, token rotation/revocation, backups, and recovery before a remote pilot.
