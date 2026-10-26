# Claude: reply-content service and App method (UX-007)

Date: 2026-10-26

For UX-008 (conversation timeline), UX-018 and the agent API:

- Go: `replycontent.NewService(ServiceOptions{})`, then
  `Extract(ctx, channel, views.MessageRef, Body) (Reply, error)`. `Reply` carries
  `Ref`, `State` (`cached`, `not_fetched`, `fetch_failed`), `Source` (`html` or
  `text`), `FromCache` and the `Result`. `BodyFromMessage` and `RefOf` map a stored
  message.
- Wails: `GetReplyContent(accountId, messageId, channel, fetch)`. With
  `fetch=false` it never downloads: a header-only message returns state
  `not_fetched` and an unavailable result. With `fetch=true` it runs the existing
  `FetchMessageBody` first. Use one channel name per view (for example `reader`):
  a newer call on the channel makes an older one that is still running fail with
  a superseded error, so the thread the user left never overwrites the one they
  selected. Treat that error as "ignore this response".
- Choice of source: clean HTML extraction wins; otherwise plain text is tried and
  the better result by status (extracted, partial, fallback) is kept, HTML on a tie.
- Bounds: results are cached in memory only (256 entries, 32 MiB), four extractions
  at a time, and a body part over 8 MiB is skipped with `input-too-large`.
  The cache is purged for an account when it is removed.
- Messages flagged S/MIME or PGP encrypted are analyzed from the stored body and
  never cached. Extraction from decrypted text is not wired; do it explicitly
  from the decrypt result if UX-008 needs it.
- HTML blocks are already sanitized with remote images blocked; do not render
  them through anything that re-enables remote content.
- The Wails bindings in `frontend/wailsjs/go` were written by hand to match the
  generator output; regenerate them with Wails on the next build and expect no
  diff beyond formatting.
