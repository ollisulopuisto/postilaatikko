# P-020 Email templates handoff

**Branch:** `codex/P-020-email-templates`  
**Worktree:** `/private/tmp/postilaatikko-p020`  
**Implementation:** `c51ac19` (pushed)

## Implemented

- Add locally stored full email templates with name, optional subject, and optional body. A subject or body is required. CRUD is exposed through Wails bindings and backed by the existing settings store.
- Add an Email templates menu and manager beside Quick replies in the composer.
- Insertion fills the subject only when the current subject is blank, then inserts the body at the current cursor. Recipient fields, sender identity, nonblank subject, and existing body content are preserved.
- Localize the user-facing interface in English and Finnish; document the behavior in the Unreleased changelog.

## Verification and limitations

- `go build ./app` passed.
- English and Finnish locale JSON parsed successfully.
- `git diff --check` passed.
- No tests were run, per session instruction. Svelte check and frontend build were not available because `frontend/node_modules` is absent in this worktree; dependencies were not installed.

Review points: try insertion into both an empty compose and an existing reply, and confirm subject/body selection restoration in rich-text and plain-text modes. This branch is a review candidate, not merged.
