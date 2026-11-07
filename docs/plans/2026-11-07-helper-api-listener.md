# Helper mailbox API listener

`postilaatikko-helper serve` runs the authenticated mailbox API on `127.0.0.1:8029` by default, beside the Send Later endpoint on `127.0.0.1:8028`. It opens one `SyncRuntime`; the runtime's SQLite database backs both mailbox reads and durable API-client grants, so the API does not open a second database or migrate independently.

The helper starts the API listener before its workers and stops it during shutdown before closing the runtime. `--sync` independently controls whether the existing header-first sync and automatic-filter workers start. On macOS, `install-agent` persists the API address, TLS identity, and allowed Host names in the LaunchAgent arguments.

Remote API binding is opt-in through `--api-listen`, `--api-tls-cert`, `--api-tls-key`, and repeated `--api-allowed-host` values. It uses the same bearer-token, revocation, capability, Host, and Origin checks as localhost. The helper's Send Later listener retains its separate `--listen` and `--tls-*` settings.

The mounted routes currently cover capabilities, account/folder/view metadata, and bounded message-header pages. Search, thread/body/attachment reads, mutations, drafts, and Pub/Sub remain follow-up service operations.
