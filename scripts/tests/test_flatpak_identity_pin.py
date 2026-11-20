import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = 'io.github.ollisulopuisto.postilaatikko.yml'

class FlatpakIdentityPinTests(unittest.TestCase):
    def test_pin_contains_renamed_source_assets(self):
        text = (ROOT / 'build/flatpak/flathub' / MANIFEST).read_text()
        self.assertNotRegex(text, r'(?m)^\s+tag:')
        pin = re.search(r'(?m)^\s+commit: ([0-9a-f]{40})$', text).group(1)
        available = subprocess.run(['git', 'cat-file', '-e', pin + '^{commit}'], cwd=ROOT,
                                   capture_output=True)
        if available.returncode != 0:
            self.skipTest('pinned source commit is not in this checkout; validate source before a Flatpak release')
        for asset in ['build/linux/io.github.ollisulopuisto.postilaatikko.desktop',
                      'build/flatpak/io.github.ollisulopuisto.postilaatikko.metainfo.xml',
                      'webview_profile.go', 'internal/platform/startup_approval.go']:
            result = subprocess.run(['git', 'cat-file', '-e', pin + ':' + asset], cwd=ROOT,
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        module = subprocess.check_output(['git', 'show', pin + ':go.mod'], cwd=ROOT, text=True)
        self.assertIn('module github.com/ollisulopuisto/postilaatikko\n', module)

    def test_release_preparation_replaces_commit_without_stale_tag(self):
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            for name in [MANIFEST, 'calculate-hashes.sh']:
                shutil.copy(ROOT / 'build/flatpak/flathub' / name, directory / name)
            # A mock transport resolves a lightweight release tag without network.
            tools = directory / 'tools'
            tools.mkdir()
            fake_git = tools / 'git'
            pin = 'a' * 40
            fake_git.write_text('#!/bin/sh\nprintf "%s\\t%s\\n" ' + pin + ' refs/tags/v-test\n')
            fake_git.chmod(0o755)
            result = subprocess.run(['bash', str(directory / 'calculate-hashes.sh'), 'v-test'],
                                    env={**os.environ, 'PATH': str(tools) + ':' + os.environ['PATH']},
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            text = (directory / MANIFEST).read_text()
            self.assertIn('commit: ' + pin, text)
            self.assertNotRegex(text, r'(?m)^\s+tag:')

if __name__ == '__main__':
    unittest.main()
