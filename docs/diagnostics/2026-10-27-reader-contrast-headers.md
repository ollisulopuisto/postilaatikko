# READER-002: contrast, quoted headers and bubble tails

User tested v0.3.17-reader-test.1 and reported unreadable black text on dark
bubbles, visible Outlook quoted headers and unclear bubble direction.

## Causes and fixes

- Reader CSS applied its foreground only to html/body. Descendants with inline
  black/white text, background colors, font color attributes and text-fill
  declarations retained their own colors. Normalize a presentation copy using
  an inert HTML template: remove color/background/text-fill/opacity/filter and
  font sizing declarations, then inherit theme colors. Keep bold, structure,
  links, code monospace and image policy. Traditional/original HTML and stored
  messages are unchanged.
- The HTML-to-text boundary map inserts line breaks around block elements.
  Header recognition stopped at the blank lines this introduced; table label
  and value cells also landed on separate lines. Allow blank lines inside a
  bounded 24-line run, require three distinct recognized labels, stop at prose,
  and keep table cells on the same logical line. Boundaries inside pre/code
  examples are rejected so literal header samples remain visible. A single From line or repeated
  From labels do not count. Extractor version reply-v3 invalidates old results.
- Rounded corners were insufficient to show direction. Add outward-pointing
  16px tails with matching solid bubble colors and border edges. Allow overflow
  on each bubble so tails remain visible; retain room at narrow widths.

## Verification

Synthetic fixtures reproduce div, br, table, nested-paragraph and name-only
header layouts. They failed before the fix and pass afterward, together with
existing extraction/service/cache tests. Ordinary From prose, duplicate labels
and original/quote-only fallbacks remain protected.

A temporary real-Chromium harness uses actual Go extractor output and authored
black/white !important, legacy font colors, opacity and filter declarations.
Dark and light checks pass for readable inherited colors, quote removal,
visible unclipped tails, narrow sizing, original disclosure and Traditional.
Frontend type/shortcut checks, targeted lint, production build and Go race/vet
results are recorded in TASKS. Browser tests do not substitute for WKWebView.

The existing Mac test workflow now also builds this task branch. It publishes
another v0.3.17 reader-test prerelease with arm64/amd64 DMGs after checks pass.
No remote-main merge is performed. See the coordination handoff about the
shared checkout's accidental local fast-forward; it does not mean origin/main
contains the published candidate.
