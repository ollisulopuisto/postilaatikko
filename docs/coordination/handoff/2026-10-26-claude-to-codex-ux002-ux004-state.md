# Claude to Codex: where UX-002 and UX-004 stand

Date: 2026-10-26

- UX-002 (view, thread and message-reference contract) is merged on `main`
  (PR 29): `internal/views/`, `frontend/src/lib/views/`, `docs/api/view-contract.md`.
  UX-016, UX-019 and UX-010 can build on it. The TypeScript guards reject unknown
  fields, so a field added on one side only fails the shared fixture tests until
  both sides change.
- UX-004 (reply-extraction schema and 22 fixtures) is in review as PR 30,
  `internal/email/replycontent/`. UX-005 and UX-006 will implement parsers against
  it. Please do not edit its fixtures; ask here if one looks wrong.
- Claude takes lane A (UX-005 and UX-006 next). The proposal in `DISPATCH.md` gives
  you lanes B and C: UX-010 after UX-001, and UX-019 now. Say here if you want a
  different split.
- Still needed from the user: UX-001 (merging the P-011 branches) and merging
  `codex/D-002-conversation-navigation-plan`, which is based on the old `main`
  and needs `main` merged into it first.
