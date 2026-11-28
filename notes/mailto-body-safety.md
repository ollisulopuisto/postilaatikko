# Mailto text body and input boundary

External OS/pending/detached mailto parsing already extracts only to/cc/bcc/
subject/body, never attachments or arbitrary headers. Both main and detached
compose setup carry body as text_body. Rich Composer.initializeFromMessage
previously only seeded html_body, dropping that text body. Plain mode worked.
Now text-only initial messages use existing plainTextToHtml fallback; existing
HTML drafts/replies remain authoritative. Body tags display literally, no HTML
interpretation/remote image/script load from link text.

Parser moved unchanged out of app.go before hardening, so it can be tested
without desktop GTK. Five-field allowlist explicit; unknown attach/attachment/
in-reply-to/references/x/from ignored. Duplicate allowed keys, including mixed
case, reject the URL. Header values lose all Unicode control characters. Body
keeps normalized LF/tab but drops other controls. Existing2KB total URL,254byte
address,998byte subject and64KB body caps retained; truncation UTF8-safe. Body
newlines now preserved rather than being removed as if header injection.
Raw remains transport-only for detached reparsing, not a compose field.

Failing-first: actual extracted composer initialization test failed text-only
body; old parser fails new controls/duplicates tests. Legacy parser and new
negative tests pass directly; full Go app tests cannot build locally without
GTK development libraries, delegated to CI. Negative cases include local-path
attach/attachment, arbitrary headers, encoded CR/LF/NUL/tab, literal HTML/script,
body newlines, oversize input and duplicate field case variants. Real Svelte
rich composer browser fixture shows escaped markup, cc, original body; confirms
no img/script DOM nodes or evil.example requests. No send invoked. JS1139pass,
34skip (browser opt-in run separately), lint/check/build pass;1existing check
warning and chunk-size advisory. Screenshot pixels inspected1100x800.

Own review traced ParseMailtoURL callers (pending OS, external, detached),
text/html initialization, cc/bcc, plaintext switches and existing HTML restore.
Internal links in message bodies separately discard query in EmailBody before
onCompose; not changed here and not the external-link bug under investigation.
No automatic send, attachment, remote fetch, app deployment or merge.

Independent review follow-up: prefilled Bcc was hidden despite being set. Added
showBcc for initial messages and history restore; restored Cc also made visible.
Existing real-composer fixture gained a Bcc visibility assertion, failed before
fix and passed afterward. Actual updated pixels inspected. Header sanitizer also
removes Unicode line/paragraph separators and bidi override/isolate controls;
negative test covers these. Malformed query escapes or literal semicolon now
reject the whole URL instead of returning only To. CI browser port collisions
fixed by unique5397 (menu PR uses5396). No duplicate fixture introduced.
