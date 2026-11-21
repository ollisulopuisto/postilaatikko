
## Security advisory fixes, 2026-11-21

Branch: `agent/security-advisory-fixes`. Owner: Instinct. Status: Review pending.

Update Go's security patch floor and vulnerable dependencies while keeping application APIs unchanged. The module-level Go advisory baseline falls from 18 IDs to zero, and npm falls from 14 affected packages to two moderate records. The old esbuild bundled by svelte-i18n remains rather than forcing a potentially breaking pre-1.0 jump or downgrading the app's translation library. Review the remaining coverage limits before merging.


<details><summary>Verification and limits</summary>

- Go minimum 1.26.0 to 1.26.11; x/net 0.61 to 0.62 and x/crypto 0.58 to 0.59, with required compatible x/* dependencies.
- Prioritize reachable stdlib HTTP/attachment paths by requiring the patched toolchain. govulncheck v1.1.4 module scan: 18 unique advisory IDs before, zero after. Two full symbol scans exceeded the local execution window, so symbol-level verification is UNKNOWN, not clean.
- npm audit --package-lock-only: 14 affected packages before, two moderate records after (svelte-i18n and its esbuild 0.19.12 dependency). No npm --force or fixes to app code.
- Same-major postcss-selector-parser 6.1.6 override scoped only to @tailwindcss/typography. ESLint/TypeScript-ESLint update within their direct major versions. Their maintained dependency graphs include internal major changes; no application-facing major upgrade is introduced.
- Frontend clean install, lint, check, build and tests pass: 1118 passed, 26 browser-gated skips. Go tests, full native build and visual/native Mac acceptance not yet run locally. No release or user mail access.
- Remaining esbuild advisory: https://github.com/advisories/GHSA-67mh-4wv8-2f99
- Own review: checked full manifest/lock/Go diffs, kept generated asset changes out, no scanner-pilot workflow changes. Devin review requested separately.

</details>

### Follow-up review, 2026-11-21

- Owner approved fixing the remaining esbuild chain. A scoped svelte-i18n override uses esbuild 0.25.12, matching Vite, while preserving svelte-i18n 4.0.1. No translation API downgrade. npm audit is now zero.
- Re-ran clean install, lint, check, build and tests after the actual resolved esbuild change: 1118 pass, 26 browser-gated skips. The translation-extraction CLI processed all Svelte source files successfully. Runtime/native Mac UI not inspected.
- Devin found stale Flatpak dependency sources. Before regeneration the five source tests fail on a missing npm archive. Regenerated node-sources.json and go.mod.yml/modules.txt using the documented pinned generator; source tests now pass 5/5 and coverage validates 465 lock entries. Full offline Flatpak build not run.
- Implementation commit before this follow-up: 40c26c0a. New commit in this branch carries the correction. Hosted exact-head CI must pass before merge.
