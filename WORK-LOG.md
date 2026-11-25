
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

## O6 selection action targeting
Approved Loki seq285 following O5/O6 sketches, under the authenticated no-repeat-confirmation grant. Checked-set state is passed through all four row render paths independently of resolved message IDs. Quick controls are removed while selection exists; contiguous checked runs are outlined separately, excluding gaps. Existing bulk callbacks and context-menu behavior remain; ActionBar anchors to a reserved final checked-row footer and clamps to viewport when offscreen. No new drag or confirmation interaction. Failing-first wiring tests 0/2 before, 2/2 after, plus geometry test. Actual row and MessageList browser regressions pass. Frontend 1129 pass, 31 browser-gated skips, lint/check/build and version check pass. No native live-mailbox run.

## 2026-11-24: headless helper account add

Owner requested accounts add without a GUI, then explicitly asked ready by morning.
Read-only report traced current main f4fbd087 before implementation; no open PR
or candidate account-add branch found in live inventory. Separate worktree.
Adds local offline setup, exclusive schema/profile lock, existing migrations and
credential writer. Existing PKCE loopback flow reused, timeout propagated to
exchange/userinfo. No device-code/OOB path, scope migration, sync/send or export.
Password input terminal-only and no echo; provider errors/logs disabled during
setup. Mail keyring-down encrypted fallback preserved. Pairing identity remains
a separate PR/design. Version2026.11.24.2 materialized metadata updated.
Own adversarial review ran before push: traced credential readers, account
identity insertion, duplicate guard, profile locks, OAuth resolver selection,
callback/token error paths and downstream Gmail/SMTP routing. Found/fixed
partial identity-insert orphan cleanup, explicit shipped-client selection and
uncancelled exchange/userinfo requests. Regression tests added for each state.
Initial missing-feature tests failed to compile; storage, OAuth and helper tests
now pass with CGO disabled; accountsetup/oauth race tests and scoped vet pass.
Helper binary builds with CGO disabled. Native GTK dependency unavailable locally,
so no GUI/native build claim. No Google/Microsoft/real-mailbox/receiver testing.
No merge. Independent post-push review requested through coordinator.

## 2026-11-24: explicit Linux file identity backend

Owner requests headless pairing identity after account-add; separate PR, no merge.
Checked main, live PRs/branches and all CreateIdentity/LoadIdentity consumers.
326 account-add is separate; no file identity duplicate. Existing Linux
install-agent unsupported, retained; macOS keyring/launch agent unchanged.
Design: explicit encrypted-file flag, default keyring. Separate identity store
uses existing AES-GCM device encryptor, private directories/files, name binding,
process lock, no-replace atomic publication. No missing-key/corruption recovery
or silent migration. Threat model limited to local OS-account boundary, not
hardware-backed protection; full profile access exposes its software key.
Pre-push own review RAN: traced init/public-key/pair/serve/client paths, keyring
consumer separation, process publication and crash states, file/type/perms,
name binding, lost-key behavior and current encryptor limitations. Fixed nested
lock hazard before testing; added root private-directory validation.
Falling-first missing-implementation compile recorded. Scheduledsend and helper
suites pass with CGO disabled; scoped vet passes; actual helper process
init/public-key/duplicate-init with no D-Bus passes. Race tests pass using an
explicit Go overlay replacing only unavailable GTK startup notification with a
no-op; no production source shim committed. Real Ubuntu service deployment,
real pairing/delivery and macOS keychain not claimed. Version2026.11.24.3 avoids
326's2026.11.24.2; both independent PRs need release-note reconciliation later.


## 2026-11-25:327 integration after326

Owner approved both merges through authenticated chat.326merged7fc00fa0 first.
Rebased327onto that main, preserving both changelog/task/log sections. No
production conflict: main.go account dispatch and identity backend changes
merged independently. Reconciled all current version fields to2026.11.25.1,
preserved326AppStream history and corrected account docs to link the now
available explicit file-identity path. Existing permission/regression fixes
retained. Own delta review compares old/new feature source and command entry
points; combined accountsetup/OAuth/scheduledsend/helper tests, scoped vet,
CGO0helper build and version checker run. No actual provider/Ubuntu service
acceptance claim. Fresh exact-head CI and independent delta review required.

## 2026-11-25: A4 Mac text shortcut ownership

Current mainaeedca2, open PRs empty, branch inventory inspected. No overlapping
shortcut change found; protected312 untouched. Palette modifier predicate rejects
physical Control on Mac while keeping Command and non-Mac Control behavior.
App supplies its existing isMacPlatform detection explicitly. Undo and Select All
return before preventDefault/mail handler when target, composed path or active
focus is editable. Composer ownership and native Edit dispatch remain unchanged.
Own adversarial review RAN: traced registry palette predicate, modifier dispatch
before input guard, native menu text Undo, composed events, modal/composer gates,
non-Mac behavior and displayed registry hints. No open findings. New regression
failed on old source; actual App dispatcher executed for all three text focus
routes and nontext mail Undo. Frontend1132pass,32browser skips; targeted tests,
lint/check and version consistency pass. Initial source proximity test failed
after a long platform default expression; moved detection to a helper, no test
weakened. Native Mac/browser editing behavior and full backend tests not run.
No workflow, dependency, schema, recipient or account changes. Not merged.
