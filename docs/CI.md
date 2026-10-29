# CI and immediate macOS installers

Pull requests always report the existing **Version consistency**, **Type check**, **Test** and **Lint** status names. Version consistency and the CI policy regression tests run for every PR. Lint is an always-run aggregate gate: a failed, cancelled or unexpectedly skipped required job fails it. No branch-protection settings need changing.

The selector reads the full Git diff against the PR base, including deleted files and both sides of renames. It does not use the GitHub changed-files API or its path-filter limit. Documentation under `docs/`, issue templates, root Markdown and LICENSE changes skip the language jobs. Frontend changes run frontend lint, type/shortcut checks, the complete Node regression suite and a production build with one npm install. Go/module/linter changes run Go lint and the complete `make test` suite with one GTK/Go setup. Mixed changes run both jobs in parallel. Unknown paths (including workflows, scripts, Makefile, VERSION and Wails configuration) select both jobs. A manual CI dispatch runs both suites regardless of paths. An invalid Git range fails the workflow.

Go compile/test jobs create a disposable `frontend/dist/index.html` fixture solely to satisfy `go:embed`; they do not need Node or a second frontend production build. No executable or fixture is uploaded from CI. Installer workflows independently build the real frontend. Go-only PRs do not check frontend behavior; frontend-only PRs do not run backend tests. Changes to shared configuration run both. New embedded UI assets or cross-language coupling should be added to the selector/tests before relying on narrower checks.

Compared with the previous full PR workflow, npm installations fall from three to one, frontend builds from two to one, and GTK/Go setup from two to one. Docs-only PRs use two short status jobs. New commits cancel superseded PR runs. These are reductions in repeated work, not measured billing or wall-clock savings; the newly enabled full frontend regression suite also takes time. The initial selector job adds one short dependency before language jobs. npm download and Go build/module caches remain enabled. Main macOS builds skip documentation-only pushes, cancel older automatic builds and cache the pinned native Wails CLI.

## Tagged releases: macOS and other platforms

Pushing a `v*` tag starts two release workflows. **Release for macOS** builds arm64 and Intel DMGs on native runners and publishes each architecture as soon as it finishes. The Apple Silicon DMG does not wait for the Intel Mac runner. Each DMG contains `Postilaatikko.app` and an `/Applications` shortcut; the corresponding ZIP is published alongside it.

The **Release** workflow builds and publishes Linux and Windows assets independently of the macOS jobs. Both workflows publish to the same GitHub Release for the tag; either may create it first, and release creation tolerates the other workflow winning the race. A release can therefore appear with whichever assets have completed so far, then gain the remaining assets as their builds finish. A normal `v*` tag still means release all supported platforms; macOS is simply no longer a blocking job in the Linux/Windows workflow.

## Build a hotfix DMG immediately

Use **Actions → macOS Build → Run workflow**, select the branch containing the intended hotfix, and choose `arm64` (Apple Silicon, default) or `amd64` (Intel). This existing artifact-only workflow does not wait for CI or a release pipeline, and manual dispatch ignores automatic push path filters. The updated workflow must first be integrated into the default branch to make its inputs available, and the selected source branch must contain the desired workflow version.

From a checkout with an authenticated GitHub CLI:

```sh
gh workflow run build-macos.yml --ref YOUR_HOTFIX_BRANCH -f architecture=arm64
gh run list --workflow build-macos.yml --branch YOUR_HOTFIX_BRANCH --event workflow_dispatch --limit 5
gh run watch RUN_ID --exit-status
gh run download RUN_ID --pattern 'Postilaatikko-macos-arm64-dmg-*' --dir hotfix-installer
```

Select the run with the expected branch and commit before watching/downloading it. Each architecture uses a native GitHub runner so the app and bundled helper target the same CPU. The artifact name includes architecture and the exact source SHA; the run summary records both. Manual runs have distinct concurrency groups, so later main pushes or another manual build cannot cancel a hotfix build. There is a 25-minute build timeout, not a guaranteed completion time: runner availability and cold caches still apply.

The workflow checks version consistency, builds the real frontend/app and bundled Send Later helper, verifies the ad-hoc app signature, packages and verifies the DMG, and retains it for 14 days. As before, this build does not run test suites. A successful installer build does not prove CI, native runtime, delivery/security acceptance or release readiness. Review CI separately and test the selected installer on-device. This path has read-only repository permissions and does not create tags, publish prereleases or trigger a normal release. It remains separate from the two tagged release workflows.

GitHub behavior references: [required/skipped status checks](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks), [manual inputs and default-branch availability](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).
