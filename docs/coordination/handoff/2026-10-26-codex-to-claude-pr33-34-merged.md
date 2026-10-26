# PR #33/#34 merged; UX-007 handoff

The user requested #33 first, then #34. Coordinator merge commits:

- PR #33 / UX-005: `74ed7ff`, source `382875cc53f166676c3e7ecd96713069791264bd`.
- PR #34 / UX-006: `7436026`, source `7e222c0659bf874c551f1e4c04b3e011edb902fd`.

Both heads had successful CI. Combined-tree verification passed:
`go test ./internal/email/... ./internal/views`, `go vet ./internal/email/...`
and `go build ./app` on macOS. Actual-mail fidelity and reader behavior remain
unverified; these PRs add parsers, not the conversation UI.

UX-007 is reserved for Claude from the user's handoff. Fetch updated main and
publish the task branch before implementation; add actual branch/worktree metadata
to TASKS.md. No UX-007 remote branch existed at the coordinator's pre-merge check.

Use the existing APIs and accepted schema:

- `replycontent.ExtractPlain(body string) Result`: exact original text slices,
  byte-based Sources, conservative unknown/fallback blocks.
- `replycontent.ExtractHTML(body string) Result`: sanitized HTML blocks, no
  source byte offsets, remote images blocked. Preserve image-blocking and CID
  behavior when wiring the reader/service.
- Both use `input-too-large`; preserve originals and fallback recovery. The
  1 MiB parser guard limits classification, while HTML fallback still sanitizes
  its input: the service must enforce its own input, concurrency and cache-byte
  bounds rather than assume the guard caps all allocation/work.

The coordinator narrowed UX-007 dependencies to UX-002/005/006. Processing already
fetched bodies and explicit existing body fetch does not require virtual-folder
refresh UI or live-device checkpoint UX-001. Those outstanding gates remain on
UX-001 and release verification; this does not waive them or authorize broad
background body/attachment downloads. UX-006 now records its actual UX-005 shared-
helper dependency. UX-008 reader still waits for UX-007.
