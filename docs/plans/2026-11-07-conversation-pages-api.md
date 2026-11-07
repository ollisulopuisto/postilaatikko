# Authenticated Conversation Pages API

**Goal:** Give the desktop API the bounded, header-only conversation pages it needs to replace local folder-list queries with the always-on mailbox service.

**Architecture:** Add an authenticated `POST /v1/conversations` operation backed by the same `message.Store` conversation queries used by the desktop. Support real-folder and unified-inbox scopes, `newest`/`oldest` ordering, and the existing `unread`, `starred`, and `attachments` filters. Return conversation summaries without message bodies. Use bounded numeric offsets for this first local API page contract and add a deterministic thread-ID tiebreaker so adjacent pages do not reshuffle when dates tie. Add a typed HTTP client method; keep desktop routing for the dependent follow-up task.

## Task 1: Define the request and response contract

Add a v1 request type containing a validated `views.Scope`, optional sort order and filter, and a bounded `PageRequest`. The response contains conversation summaries, a next cursor, and a completion flag. Accept only `folder` and `all_inboxes` scopes in this operation. Validate account ownership for folder scope, sort and filter values, and cursor bounds before reading the store.

## Task 2: Reuse the existing conversation query implementation

Add a separate conversation-list operations contract and mount it from the always-on runtime's existing catalog/store adapter. Call `message.Store.ListConversationsByFolder` or `ListConversationsUnifiedInbox`; do not copy SQL or filtering semantics into the API handler. Set account/folder identifiers on folder summaries where the store query does not populate them. Ensure the wire response does not contain `Messages` or body content.

## Task 3: Mount and consume the authenticated route

Mount `POST /v1/conversations` under the existing `mail.read` capability. Add `Client.ListConversations` using the existing authenticated HTTP transport and v1 envelope. Keep current desktop reads unchanged in this task so API contract work remains independently reviewable.

## Task 4: Record the service boundary

Document the endpoint and its scope, filters, pagination, header-only guarantee, and capability in `docs/API.md`. Update `CHANGELOG.md` and `TASKS.md`; mark HOSTED-SERVER-012 integrated at merge `dc2553f` and record verification for this task.

## Verification

- Run `gofmt` on changed Go files.
- Run `go build ./app ./internal/automation/... ./internal/alwayson/...` from the task worktree.
- Run `git diff --check`.
- Do not run or add tests unless separately requested.

## Follow-up dependency

The next desktop-client task can call this API from `GetConversations` and `GetUnifiedInboxConversations`, map summaries to the existing Wails model, and preserve on-demand/prefetch body policy. It should route only scopes represented by this endpoint and keep unsupported virtual-folder paths intact until their API contract is added.
