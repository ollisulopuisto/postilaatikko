# Remote S/MIME and PGP feature gates

**Goal:** Make the remote desktop's unsupported local-only crypto integrations safe and explicit.

## Scope

- Guard exported S/MIME and PGP Wails bindings for key/certificate CRUD, policies, lookups, recipient discovery, file pickers/import, and key-server configuration.
- Return a clear unsupported-mode error before local crypto stores, credential stores, file dialogs, or direct key-discovery network calls are used.
- Keep boolean composer availability checks safe and false in remote mode.
- Preserve local-helper behavior. Do not add helper crypto APIs or change local encryption/signing.

## Acceptance

- Each exported crypto binding either fails explicitly in remote mode or safely reports unavailable for a boolean capability query.
- Remote calls cannot dereference omitted crypto/credential stores or initiate local crypto-provider work.
- Local mode continues to delegate to existing implementations.
- The affected app and helper packages build and `git diff --check` passes.
- No tests are added or run for this slice.

## Verification limits

- Remote S/MIME/PGP remains unavailable until helper-owned key custody and crypto operations are designed.
- Existing helper draft/send APIs continue to reject encrypted content; live encrypted-mail behavior is not verified here.
