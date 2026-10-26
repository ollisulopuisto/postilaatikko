# Claude: HTML extraction API (UX-006)

Date: 2026-10-26

- `replycontent.ExtractHTML(body string) Result` is the HTML extractor, next to
  `ExtractPlain` (UX-005). Both return the same `Result`; UX-007 chooses between
  them and caches.
- HTML blocks have `Format: html`, no `Source` (the DOM keeps no byte offsets),
  and content that is already sanitized with the application's email policy and
  has remote images blocked. The original URL stays in `data-original-src`, so
  the reader can restore an image on request with the existing mechanism. Do not
  run the block through another renderer that re-enables remote content.
- Quote detection is by client markers: Gmail `gmail_quote`/`gmail_signature`,
  Outlook `divRplyFwdMsg` (the rest of the message after it is quoted), Apple and
  Thunderbird `blockquote type=cite`, Thunderbird `moz-cite-prefix`, Yahoo
  `yahoo_quoted`. A `blockquote` with no marker is quoted only when a "wrote:"
  line precedes it or no new text follows; otherwise it is shown as unknown.
- A forwarded message is never collapsed: from the marker to the end it is one
  unknown block with the warning `forwarded-content`.
- Bodies over 1 MiB, or nested deeper than 256 elements, come back as one
  sanitized unknown block with `input-too-large`.
- New fixtures: `gmail_html_signature_en`, `gmail_html_forwarded_en`,
  `thunderbird_html_reply_en`, `list_cid_html_en`. They are additive.
