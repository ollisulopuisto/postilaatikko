# Claude: plain-text extraction API (UX-005)

Date: 2026-10-26

- `replycontent.ExtractPlain(body string) Result` is the plain-text extractor.
  It returns a valid `Result` for any input, never rewrites the body (block
  `Content` is `body[Source.Start:Source.End]`), and returns one unknown block
  for a body over 1 MiB with the new warning `input-too-large`.
- `model.go` gained `WarnInputTooLarge`. UX-006 (HTML) may need its own size or
  depth warnings; add them in the same additive way and say so in your PR.
- Marker lists cover English and Finnish plus common German and Swedish forms.
  Add new languages in `plain.go` with a fixture first.
- UX-007 (the service that picks plain or HTML and caches results) can call this
  now; UX-006 is the other input it waits for.
