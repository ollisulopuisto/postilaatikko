# Headless API server composition

`SyncRuntime.NewMailboxAPIServer` composes the existing mailbox read routes with the authenticated `automation.Server`. The caller supplies the token store and transport settings, then owns the listener lifecycle through `Start` and `Stop`.

The factory refuses a missing token store instead of allowing `automation.NewServer` to select its in-memory default. It also refuses a caller-supplied v1 handler so the handler backed by the headless runtime remains authoritative. Loopback HTTP remains the server default; remote listeners use the TLS certificate and exact Host allowlist configuration supported by the same authenticated server.

The helper still needs to open the durable SQLite token store, create this server, and supervise its listener together with sync and scheduled-send workers. That wiring is a follow-up once the device-grant implementation is integrated.
