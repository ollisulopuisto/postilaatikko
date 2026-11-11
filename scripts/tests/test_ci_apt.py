import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'ci_install_linux_deps.sh'

class AptFallbackTests(unittest.TestCase):
    def run_script(self, fail_all=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'sudo').write_text('''#!/bin/sh
printf '%s\\n' "$*" >> "$APT_LOG"
if [ "$FAIL_ALL" = 1 ]; then exit 1; fi
case "$*" in *sources.list=*) exit 0;; esac
exit 1
''')
            (root / 'sudo').chmod(0o755)
            script = root / 'script.sh'
            script.write_text(SCRIPT.read_text().replace('. /etc/os-release', 'ID=ubuntu; VERSION_CODENAME=jammy'))
            env = dict(os.environ, PATH=f'{root}:{os.environ["PATH"]}', APT_LOG=str(root/'log'), FAIL_ALL=str(int(fail_all)))
            result = subprocess.run(['bash', str(script)], env=env, capture_output=True, text=True)
            return result, (root/'log').read_text() if (root/'log').exists() else ''

    def test_fallback_retries_same_packages_and_enforces_update_success(self):
        result, log = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('APT::Update::Error-Mode=any', log)
        self.assertIn('Dir::Etc::sources.list=', log)
        self.assertIn('libgtk-3-dev libwebkit2gtk-4.1-dev', log)
        self.assertNotIn('--allow-unauthenticated', log)

    def test_both_mirrors_failing_remains_failure(self):
        result, _ = self.run_script(True)
        self.assertNotEqual(result.returncode, 0)
