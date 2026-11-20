# Helper-owned Gmail filter publication API

This slice adds an account-owning helper path. It is not a completed desktop feature: consent exchange and desktop routing are still absent, and the desktop remains fail-closed for every helper connector.

## Operations

`POST /v1/filters/gmail` requires mail.read. Context, bounded preview and status are reads. Preparation reserves the stopped source, publication changes future Gmail filtering, cancellation releases an unattempted source, and reconciliation can update the local journal. Those four operations additionally require both mail.write and account.manage.

Native payloads contain one explicit Gmail query and supported label actions. Local substring criteria are not translated. User-label choices are validated against the selected helper account, system-label injection is refused, archive/Trash conflicts are refused, and the preview reads one 51-ID page, showing at most 50 plus truncation. It never changes historical mail.

The helper checks the explicit settings scope in its own canonical Mail credentials. Its new settings-token source is separate from ordinary search access. Tokens stay helper-local. The refresh path retains omitted refresh tokens and preserves old scope metadata when the response omits scope; explicit scope reductions are saved and settings access is then refused.

## Ownership and recovery

The durable journal binds account email/OAuth project and source/payload fingerprint. Each provider operation reads Gmail profile with the same pinned token used for its following request. An unknown create never replays. Reconciliation verifies one candidate with the journal's original baseline. Account/project/profile changes block publication and reconciliation.

Helper filtering already runs durable jobs. Preparation rejects pending, processing and outcome-unknown jobs within its journal transaction. The guard blocks enabling, edits, deletion and later job enqueue. No offline status or failed probe is used as proof of executor quiescence.

Only an unattempted plan can cancel. Published sources remain guarded, reverse transfer and provider deletion are not implemented. Old unbound records fail closed.

## Checks and remaining work

Tests use fake helper credentials/provider HTTP, never live Google. They cover selected helper tokens, no credential output, missing grants without provider calls, preview bounds, exact fingerprint, one uncertain create, status and read-only recovery, mismatched identity/account, job-state refusal and cancellation. Route tests cover both permissions, strict bounded JSON and credential-field rejection. The actual client tests prove failed create is not replayed. The production mount and route-coalescing classification tests cover the new route.

Full alwayson, automation/client/v1, scheduled-send, helper command and affected dependencies passed with CGO disabled; affected vet and version consistency passed. Race execution was blocked at compile time by absent native GDK/GLib libraries and is not claimed passed. No native macOS, real Studio or live Google acceptance.

Next: selected-account expiring helper-owned consent exchange, then workspace-pinned desktop routing using the native review UI. Never obtain helper refresh tokens on the desktop or silently fall back to desktop credentials.
