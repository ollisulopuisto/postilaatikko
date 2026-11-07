# Hosted API message headers

This slice adds the first bounded message-list read to the headless mailbox API. It reuses the SQLite header store and returns `MessageHeader` records, which contain envelope metadata, snippet, and flags but no message body.

## Endpoint

`GET /v1/messages/headers?accountId={id}&folderId={id}&limit={1..100}&cursor={offset}`

- `accountId` and `folderId` are required. The folder is checked against the account before reading.
- `limit` defaults to 50 and is capped at 100. A page fetches at most one extra header to determine whether another page exists.
- `cursor` is an offset from zero, capped at 1,000,000. `nextCursor` is present only when another page exists; `complete` is false in that case.
- Rows sort by date descending with message ID as the deterministic tie-breaker.
- The route requires `mail.read` and is mounted by the headless runtime handler; the enclosing automation server supplies authentication and transport protections.

The account/folder catalog API constructor remains available and now returns the combined mailbox-read handler, preserving its prior behavior while adding this route.
