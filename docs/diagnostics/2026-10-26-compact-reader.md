# READER-001: compact conversation reader and detached windows

Based on main `0ef36e4`, after UX-008/009. Implemented on
`codex/READER-001-compact-conversations`; this document does not mean merged or released.

## Behavior

- Double-click a message-list row to open its thread in a native reader window.
  Single-click keeps existing selection behavior. Nested action/checkbox controls
  cannot open windows by accident. Errors are shown in a toast.
- The reader process has its own selection/layout and an authenticated IPC
  connection to the main process. It starts no second database or sync engine.
  Parent handles on-demand bodies, attachments, mail actions and detached replies.
  Saving reader preferences never overwrites the main window's layout.
- Conversation mode uses directional bubbles, with all configured sender identities
  on the right and peers on the left. A two-person thread without a configured
  sender uses stable participant sides. Consecutive messages share a sender label;
  group threads still identify each sender run. Dates remain visible; full headers
  are a disclosure. Attachments and security notices remain accessible.
- Signature/quotation fold rows and source buttons are absent in conversation
  mode. A small ellipsis opens the complete original; Traditional retains the
  full header/source presentation. Ambiguous extraction still shows complete text.
- Fixed overlapping Conversation/Traditional labels caused by the dark theme's
  fixed width on all toolbar buttons. Matched iframe color schemes to prevent
  a forced white canvas in dark bubbles.

## Library evaluation and reuse

Reviewed primary upstreams:

| Library | Fit and outcome |
| --- | --- |
| [web-ridge/email-reply-parser](https://github.com/web-ridge/email-reply-parser) | Adopted at `95e2a82fa6bd` (MIT); Go, no transitive dependencies. Removes unprefixed addressed/date reply history in the local test. Not a complete HTML or Finnish signature solution. |
| [recapco/emailreplyparser](https://github.com/recapco/emailreplyparser) | Go fragment API; inspected source reverses text and notes combining-character and 64 KiB scanner-line limitations. Not adopted. |
| [mailgun/talon](https://github.com/mailgun/talon) | HTML quotations and signatures; Python runtime, with optional scientific/ML dependencies. Does not fit the current standalone Go app as directly. |
| [crisp-oss/email-reply-parser](https://github.com/crisp-oss/email-reply-parser) | Node plain-text parser with locale support. Keeping extraction in Go also serves future API clients consistently. |

The Go library proposes boundaries only. Accept only an exact prefix followed
by a dated/addressed `On … wrote:` attribution, without inline quoted lines.
Never use its rewritten output as stored mail. Our segmenter retains original
source offsets and handles Finnish/inline replies. The screenshot's standalone
em-dash signature requires a separated, short footer with contact information;
a prose divider alone stays visible.

For unmarked HTML, map rendered text boundaries back to cloned original DOM
ranges. Preserve links, formatting and image nodes, then sanitize through the
existing remote-image-blocking policy. The library adapter has a 64 KiB / 2,000-line limit; HTML splitting has a 32-block limit. Larger/uncertain content stays visible under the existing fallback rules. The
extractor version is `reply-v2`, invalidating old cache entries. The upstream
MIT notice is included under `frontend/public/licenses/` for packaged builds.

## Verification

- New screenshot-style plain/HTML cases failed before the fix, then passed.
- Extraction tests include retained inline answers, meaningful dashes, markup,
  links and blocked remote images. Existing fixture/service/cache tests pass.
- Reader bridge tests verify the operation allowlist, argument types/count/null,
  getter/error-only return handling and parent error propagation. Existing IPC
  authentication/socket tests and app tests pass with race detection on macOS.
- Frontend type/shortcut checks, targeted lint and production build recorded in
  TASKS. Regenerated Wails bindings retain GetReplyContent; generated UIState
  construction required a small save-adapter correction.
- Temporary real-Chromium harness with synthetic mail and actual Go extraction:
  reader IPC bootstrap, left/right bubbles, compact headers, hidden signature and
  source chrome, mode-label sizing, original disclosure, Traditional fallback,
  and readable dark iframe rendering. Actual row mouse events verify single vs
  double click and nested-control exclusion. Harness files removed afterward.

## Device and integration checks still needed

- Packaged Wails/macOS: open two reader windows, change the main selection, close
  a child without closing main, quit main, reply, view encrypted mail, and save
  attachments. Browser tests cannot validate native window creation or dialogs.
- IPC retains its existing 10 MiB frame limit; responses above 9 MiB return an
  explicit error. Very large threads need a later paged reader transport.
- Opening server-only search results requires their local conversation to finish
  downloading first. A missing local conversation returns an actionable error.
- Main-window changes refresh a detached reader on refocus; continuous cross-
  window push updates and the full main-window shortcut shell are follow-ups.
- REL-001 remains a separate review branch. When integrating both branches,
  regenerate bindings from the combined Go source: retain ReaderApp,
  GetReplyContent and scheduling operations. Preserve tagged shortcut behavior
  before promoting a replacement release.
