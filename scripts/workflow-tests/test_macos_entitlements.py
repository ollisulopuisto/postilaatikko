"""Hardened-runtime entitlement guards, run with uv --no-project --with pyyaml."""
import subprocess
import sys
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ['build-macos.yml', 'release-macos.yml']
ENTITLEMENTS = 'build/darwin/entitlements.plist'


def jobs(name):
    return yaml.safe_load((ROOT / '.github/workflows' / name).read_text())['jobs']


class EntitlementWorkflowTests(unittest.TestCase):
    def test_repo_entitlements_cover_repo_usage_descriptions(self):
        result = subprocess.run(
            [sys.executable, str(ROOT / 'scripts/check_entitlements.py'),
             '--info', 'build/darwin/Info.plist', '--entitlements', ENTITLEMENTS],
            cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_hardened_app_signing_passes_entitlements(self):
        for name in WORKFLOWS:
            for job in jobs(name).values():
                for step in job['steps']:
                    run = step.get('run', '')
                    if 'codesign' in run and '--options runtime' in run and '.app' in run:
                        self.assertIn(f'--entitlements {ENTITLEMENTS}', run,
                                      f'{name}: hardened app signing without --entitlements')

    def test_packaged_entitlements_guard_follows_signing(self):
        for name in WORKFLOWS:
            for job_name, job in jobs(name).items():
                steps = job['steps']
                for index, step in enumerate(steps):
                    run = step.get('run', '')
                    if 'codesign' not in run or '--options runtime' not in run or '.app' not in run:
                        continue
                    guard = [
                        later.get('run', '') for later in steps[index + 1:]
                        if 'check_entitlements.py' in later.get('run', '')
                        and 'codesign -d --entitlements' in later.get('run', '')
                    ]
                    self.assertTrue(
                        guard,
                        f'{name}:{job_name}: no packaged-entitlements guard after hardened signing')

    def test_all_shell_still_parses(self):
        for name in WORKFLOWS:
            for job in jobs(name).values():
                for step in job['steps']:
                    if 'run' in step:
                        subprocess.run(['bash', '-n'], input=step['run'], text=True, check=True)


if __name__ == '__main__':
    unittest.main()
