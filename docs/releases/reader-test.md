A test build of the compact conversation reader, built on GitHub. Choose
`arm64` for Apple Silicon (including Mac Studio), or `amd64` for Intel Macs.
Open the DMG and drag Postilaatikko onto Applications.

Changes in this update:
- Consistent sender sides in inbox/virtual views, including historical configured identities and multi-address correspondents.
- Gmail MIME reactions and recognizable Gmail/Outlook fallback notifications display as grouped emoji chips on their exact target message. Click to inspect the source notifications; Traditional keeps all originals. Unknown targets/templates remain ordinary emails.
- Strip short signatures before long unprefixed quoted replies.
- Adds an additive local reaction-metadata cache table; routine sync stays header-only and stored bodies are unchanged.

Included previous fixes:
- Normalize authored inline text/background colors so conversation bubbles remain readable in dark and light themes.
- Recognize spaced Outlook From/Date/To/Subject quoted headers, including table layouts.
- Add visible left/right speech-bubble tails.

Included reader work:
- Double-click a message-list row to open an independent reader window.
- Directional conversation bubbles, fewer repeated headers, and originals/details on demand.
- Better signature and quoted-history removal using a guarded Go reply-parser library.
- No signature/source control rows in conversation mode; Traditional remains available.
- Preserve the previously tagged Send Later code and message-focused shortcuts.

This is a prerelease for Mac/WKWebView testing. Send Later's real-delivery,
pairing/security and recovery acceptance gates remain open. See `docs/diagnostics/2026-10-26-compact-reader.md`
and `docs/diagnostics/2026-10-26-reader-smoke-checklist.md` in the source for checks.
