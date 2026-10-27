# READER-003: sender sides, reaction chips and signature boundaries

Based on published reader-test.2 (`279baf7`) in an isolated task branch. No remote
main merge is implied by this implementation or its test release.

## Sender alignment

The existing fallback placed everything left when it could not match an owned
address and found more than two distinct addresses. That reproduces a historical
sender plus a correspondent using two addresses. Display-address vs bare-address
forms also failed matching. A working two-address Gmail example does not prove
a folder-specific renderer: both use the same component and differed in inputs.

Normalize display/bare address forms. A reader-only backend operation includes
inactive configured accounts and their historical aliases without enabling them
for composing or sending. Match known own identities on the right, others left.
For still-unconfigured historical threads, use the first speaker as a stable
visual anchor rather than make every bubble left. This last rule is presentation
only, not authentication or proof that the first sender is the user. Layout uses
the presented thread, not the subset exposed by single-message Focus mode.

Actual mail/account data from the screenshot device was not available for direct
verification. The local expected DB path had no account tables. Root causes above
are reproduced source-level cases; verify the user's affected Inbox thread in
WKWebView and configure any still-missing historical sender aliases if needed.

## Reactions

Primary references: [Google reaction format](https://developers.google.com/workspace/gmail/reactions/format),
[RFC 9078](https://www.rfc-editor.org/rfc/rfc9078.html), and
[Outlook fallback behavior](https://support.microsoft.com/en-us/outlook/mail/reactions-in-microsoft-outlook).
The Outlook x-ms-reactions header controls permission; it is not itself a reaction.

- During existing body downloads, parse Gmail's transfer-decoded reaction JSON
  MIME part (version 1, one recognized emoji, exactly one In-Reply-To ID), or a
  single-emoji RFC reaction part. Attachment parts never count. Duplicate,
  malformed, unsupported-version and invalid-encoding metadata stay ordinary.
  Emoji lookup uses pinned MIT GoMoji v1.4.1; unknown emoji remain uncollapsed.
- Cache MIME metadata atomically with body updates in an additive migration 53
  table with message-delete cascade. No additional routine FETCH or network
  service is introduced. On-demand and batch body downloads both use this path.
- For previously cached Gmail/Outlook fallback bodies, accept only a complete
  recognized notification sentence with matching sender name and emoji. Ordinary
  emoji replies, digests, extra authored prose and unknown templates stay visible.
  Exact In-Reply-To or last References target is required; never guess the
  preceding row. Invalid cached MIME metadata cannot become a fallback reaction.
- Resolve target IDs within the same account. Missing targets, attachment-bearing
  messages and encrypted/signed messages remain whole. Deduplicate copies by RFC
  ID, group by target and emoji, retain sender names, and show a chip/count on the
  target. Clicking reveals the original notifications. Traditional shows all raw
  message cards. Reply defaults skip folded notifications; visible-card keyboard
  navigation skips hidden cards. Main thread operations retain original IDs.

This implements reading existing reactions, not sending/changing them. Native
Exchange-only reaction state that never produces an email is not exposed by
IMAP; this handles Outlook's delivered fallback notification emails.

## Signature/history extraction

The original signature scan included an unprefixed old message in its line count,
so a three-line signature exceeded the 15-line bound. The library could also
stop at that signature before reaching the quote header. Validate strong
attribution boundaries independently through the existing reply parser, then
revisit the authored prefix for signatures. Preserve inline answers and protected
code blocks. Bound library candidate attempts and retain large/ambiguous text.
Extractor version reply-v4 invalidates prior cached results.

## Verification

- Red-first alias/address-form and long-history signature tests, then green.
- Structured Gmail JSON, emoji/skin-tone validation, invalid version/target,
  attachment exclusion, exact notification text, parser transfer encoding,
  single-part fallback, metadata round trip, invalid-metadata suppression and FK
  cascade tests pass. Migration replay and existing sync/store suites pass.
- Pure UI tests cover exact target vs previous row, account isolation, missing
  targets, protected messages, copy deduplication and ordinary emoji replies.
- Race tests cover reactions/replycontent/message/database/sync and reader App
  methods. Frontend check, targeted lint, production build, vet and combined Go
  build results are in TASKS.
- Temporary real Chromium harness (synthetic mail, actual Go extraction and
  notification parser output), dark and light: aliases on correct sides, grouped
  chip on exact parent despite intervening message, reversible source disclosure,
  unresolved reaction stays visible, signature/history stripped, original retained
  after expanding Traditional card, narrow layout, no page errors.

Native WKWebView, actual provider MIME variants and the affected live Inbox thread
remain device checks. Conservative unknowns deliberately stay as normal emails.
