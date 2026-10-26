# View, thread and message-reference contract

Task UX-002 of the D-002 conversation, navigation and agent API plan
(`docs/plans/2026-10-26-conversation-navigation-agent-api.md`, section UX-002).

This is the data contract that the list, the reader, the action layer and the
agent API share. It describes what a view contains and how to refer to a
message. It does not change how threads are built, how a view is queried or how
anything is rendered.

- Go: `internal/views/model.go`
- TypeScript: `frontend/src/lib/views/types.ts`, runtime checks in `guards.ts`
- Fixtures both sides check: `internal/views/testdata/`

## Four independent choices

| Choice | Values | In this contract |
| --- | --- | --- |
| Scope | all inboxes, real folder, saved virtual folder, search | Go and TypeScript |
| Grouping | threads (people is reserved) | Go and TypeScript |
| Reader | conversation, traditional | TypeScript only |
| Layout | drawer closed/open/docked, thread focus | TypeScript only |

Changing the reader or the layout never changes thread membership, flags,
account identity or scope. Reader and layout are UI state, so agents never see
them. `ViewSpec` is scope plus grouping; `Presentation` is reader plus layout.

## References

Every message and thread is addressed with its account.

```json
{ "accountId": "acc-a", "messageId": "m-101" }
{ "accountId": "acc-a", "threadId": "thr-100" }
```

Two accounts can hold copies of one email, and one account can hold several
copies of it (an Inbox copy and an All Mail copy). Each copy is its own
message, even when the RFC `Message-ID` header (`rfcMessageId`) is identical.
Presentations may collapse known duplicates, but a mutation target always names
the exact messages. The fixtures `thread_duplicate_copies.json` and
`page_all_inboxes_partial.json` cover both cases.

`MessageRef.Key()` and `messageKey()` give a string that is unique per account
and message, with ids escaped so a `/` inside an id cannot cause a collision.
Keys are local helpers. Do not store them or send them between processes.

## Scope

```json
{ "kind": "all_inboxes" }
{ "kind": "folder", "accountId": "acc-a", "folderId": "fld-a-inbox" }
{ "kind": "virtual_folder", "virtualFolderId": "vf-1" }
{ "kind": "search", "query": "from:alice", "within": { "kind": "all_inboxes" } }
```

- All inboxes is the Inbox of every enabled account, resolved when the query runs.
- A scope has exactly the fields its kind needs and no others, so one scope has
  one representation. `Validate` (Go) and `scopeError` (TypeScript) reject the
  rest, and `testdata/scopes.json` holds the shared valid and invalid lists.
- `within` limits a search. Absent means all inboxes, and it is never another search.
- `ScopeKey` returns a stable identifier. A search with no `within` equals a
  search within all inboxes.

What a search expression means belongs to UX-003, not to this contract.

## Matched messages and conversation context

A view lists threads, and a thread can contain messages outside the view. An
Inbox result can show related Sent messages so the conversation makes sense.

- `matched: true` marks the messages that are in the scope.
- Other messages in the thread are context.
- `ThreadSummary.matched` names the matching messages. `messageCount` also
  counts context. `ThreadView.matchedCount` is the number of matched messages.

An action on a view applies to matched messages unless its target says
otherwise. A context message must never be archived or deleted just because it
is shown. UX-016 defines the target types; it uses `MessageRef` from here.

## Completeness

Each message has a `contentState`:

| Value | Meaning |
| --- | --- |
| `cached` | The body is downloaded and readable offline. |
| `not_fetched` | Only the header is stored. Reading needs an explicit fetch. |
| `fetch_failed` | A fetch ran and produced no usable body. |

`completeness` counts them. It is complete only when every body is cached. A
summary built from an incomplete thread must say so. `CompletenessOf` (Go) and
`completenessOf` (TypeScript) compute it, and both are checked on the same
fixtures. Reading a thread never fetches a body by itself.

A list page has its own `complete` flag. It is false when an account failed or
the list was cut short, and each failure appears in `errors`:

```json
{ "complete": false, "errors": [{ "accountId": "acc-c", "code": "account_unreachable", "message": "IMAP connection failed" }] }
```

## Cursors

`nextCursor` is an opaque string. A client passes it back unchanged and never
parses it.

- A cursor is bound to the scope it was issued for. `DecodeCursor` returns
  `ErrCursorScope` for another scope, so a cursor from one view cannot read
  another.
- A malformed cursor or an unknown version returns `ErrCursorInvalid`.
- The position is "after this thread, in latest-date order". An implementation
  must break ties between equal latest dates by account id, then thread id, so
  the order is total. That is what lets a page continue from the cursor without
  repeating or skipping threads. This contract only defines the position; the
  query that honors it is UX-003.

## Ordering

`ThreadView.messages` is chronological, oldest first. `latestDate` is the date
of the last message. List pages are ordered by latest activity, newest first.

## Compatibility

New fields are additive. The TypeScript guards reject unknown fields, so a
field added on one side only fails the shared fixture tests on the other side
until both are updated. UX-004 and UX-007 add authored-content blocks to a
thread message as a new optional field.

## Checks

```sh
go test ./internal/views
cd frontend && node --test tests/viewContract.test.mjs
```
