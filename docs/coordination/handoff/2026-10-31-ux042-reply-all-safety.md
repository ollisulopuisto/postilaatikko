# UX-042 Reply All and recipient repair handoff

**Branch:** `codex/UX-042-reply-all-safety`  
**Worktree:** `/private/tmp/postilaatikko-ux042`  
**Implementation:** `4118ac6` (pushed)

## Implemented

- Preserve explicit `reply`, `reply-all`, or `forward` mode as local draft metadata. It is not serialized as a mail header. Old drafts without this metadata retain the existing fallback inference.
- Show each pending send separately with sender identity and the full To/Cc/Bcc list; label Reply All explicitly.
- Add **Edit recipients** during the Undo Send window. It atomically cancels the still-pending row, reopens the associated draft under its original account, and focuses To. Existing Cancel still cancels and restores the draft.
- Disable cancellation/editing once dispatch starts and explain that delivery may already be underway. An unsuccessful or ambiguous dispatch reports that delivery may already have happened while keeping the draft available.
- Add English and Finnish strings.

## Verification and limitations

- `go build ./app` passed.
- `git diff --check` passed.
- English and Finnish locale JSON parsed successfully.
- No tests were run, per session instruction. The frontend checker/build could not run because `frontend/node_modules` is absent in this worktree; dependencies were not installed.

Review points: verify the compact pending-send card at narrow window widths, confirm the restored To field receives focus, and exercise the dispatch/cancel race on a configured account. This branch is a review candidate, not merged.
