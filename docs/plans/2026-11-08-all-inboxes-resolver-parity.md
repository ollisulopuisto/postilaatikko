# All Inboxes Resolver Parity

## Goal

Use the same enabled-account Inbox folder selection for All Inboxes helper reads, local fallback reads, conversation counts, and unread counts.

## Scope

- Reuse resolved folder IDs in desktop fallback list/count paths, including unread counts.
- Add focused tests covering mapped custom-type folders and the default Inbox fallback.
- Record the user-visible All Inboxes behavior in the changelog.

## Acceptance

- The helper and local fallback select the same Inbox-typed folder per enabled account, including when several Inbox-typed folders are cached.
- Conversation and unread counts use the same selected folder IDs as the list.
- Disabled accounts and absent mappings retain existing behavior.
- Focused Go tests and `git diff --check` pass; relevant package build passes.

## Verification

- `go test ./app ./internal/alwayson ./internal/message`
- `go build ./app ./cmd/postilaatikko-helper`
- `git diff --check`

## Risks

- Inbox has no account path-mapping field in the current account model; selection follows the existing special-folder resolver. Unread count semantics use cached per-folder counts to match sidebar behavior.
- The desktop can be offline from the helper; its fallback must honor mappings too.
