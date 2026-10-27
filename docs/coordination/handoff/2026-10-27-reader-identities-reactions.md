# READER-003 ownership

Codex owns viewer identity/layout and reaction presentation, replycontent signature
boundaries, and reaction metadata parsing/storage on the existing explicit body
fetch path. User requested inbox/folder consistency, Gmail/Outlook reactions and
signature-plus-attribution fixes. Worktree /private/tmp/postilaatikko-reader003;
branch codex/READER-003-identities-reactions. Based on latest origin/main plus
published reader-test.2 (279baf7). Do not infer integration from local main.

Plan: reproduce multi-address/no-own-identity layout and signature-before-history
cases; fix lossless boundaries; parse structured Gmail MIME and conservative
fallback notifications; resolve exact In-Reply-To/reference targets, leave
unresolved/ambiguous messages visible; render reversible reaction chips in
conversation mode; keep Traditional original messages; verify and GitHub-build.
