# Compact conversation reader implementation plan

**Goal:** Open threads in independent windows and make conversation mode read like a chat.

**Architecture:** Reuse the conversation renderer and main process mail services through authenticated local IPC. Each detached reader owns its UI state and does not start another sync engine. Keep original/Traditional rendering available through a small disclosure, without repeated source/signature controls. Evaluate established reply parsers against the fixture corpus before adopting one.

**Tech stack:** Go, Wails v2, Svelte 5, existing authenticated IPC and sanitized EmailBody.

1. Inspect libraries: Go recapco and web-ridge; Python Talon; Node Crisp. Record language/HTML support, licenses, fixture suitability, and runtime cost. Choose based on tests, preserve source and unknown text.
2. Add regression cases in internal/email/replycontent for standalone em-dash signatures, unprefixed Apple-style history, and inline replies that must remain. Run Go tests before and after extraction changes. Bump extractor version if behavior changes.
3. Add a pure presentation helper and frontend tests for self vs peer alignment and sender-run labels. Render compact conversation headers and bubbles in ConversationViewer; retain Traditional headers, attachments and crypto indicators. Remove signature fold rows and source controls from conversation mode; keep originals via disclosure. Fix reader toggle sizing.
4. Add app detached-reader IPC dispatch with an explicit operation allowlist and argument validation, tests for rejected operations/types and error propagation. Add --reader entrypoint, ReaderApp and reader.html bootstrap; reuse ConversationViewer over that bridge. Wire row double-click while excluding interactive descendants.
5. Run focused Go tests/race, backend build, frontend type/shortcut checks, lint on changed files and production build. Document device-only checks: actual secondary windows, WKWebView focus, alignment/stripping on real mail, Traditional/original, attachments and image privacy.
6. Update TASKS with verification, limitations and review commit; push task branch without merging main.
