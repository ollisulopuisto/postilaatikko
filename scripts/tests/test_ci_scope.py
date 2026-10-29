"""Regression checks for selective CI; stdlib only, no project install required."""
import importlib.util
import pathlib
import subprocess
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / 'ci_scope.py'
spec = importlib.util.spec_from_file_location('ci_scope', SCRIPT)
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


class ScopeTests(unittest.TestCase):
    def test_change_scopes(self):
        cases = [
            ([], (False, False)),
            (['TASKS.md', 'docs/plans/change.md'], (False, False)),
            (['.github/ISSUE_TEMPLATE/bug.yml'], (False, False)),
            (['frontend/src/App.svelte'], (True, False)),
            (['frontend/package-lock.json'], (True, False)),
            (['frontend/wailsjs/go/models.ts'], (True, False)),
            (['internal/message/query.go'], (False, True)),
            (['go.mod', 'go.sum'], (False, True)),
            (['docs/example.go'], (False, True)),
            (['docs/notes.md', 'internal/reader/foo.go'], (False, True)),
            (['frontend/src/App.svelte', 'app/state.go'], (True, True)),
            (['.github/workflows/ci.yml'], (True, True)),
            (['scripts/ci_scope.py'], (True, True)),
            (['Makefile'], (True, True)),
            (['wails.json'], (True, True)),
            (['VERSION'], (True, True)),
            (['some-new-config.toml'], (True, True)),
            ([f'docs/note-{i}.md' for i in range(350)] + ['late-config.toml'], (True, True)),
            (['frontend/funny\nname.ts'], (True, False)),
        ]
        for paths, expected in cases:
            with self.subTest(paths=paths):
                self.assertEqual(policy.classify(paths), expected)

    def test_git_diff_handles_deletions_renames_and_newlines(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)

            def git(*args):
                return subprocess.check_output(['git', '-C', directory, *args], text=True).strip()

            git('init', '-q')
            git('config', 'user.email', 'ci@example.test')
            git('config', 'user.name', 'CI tests')
            (root / 'docs').mkdir()
            (root / 'frontend').mkdir()
            (root / 'docs/note.md').write_text('old')
            (root / 'old.go').write_text('package old')
            git('add', '.')
            git('commit', '-qm', 'base')
            base = git('rev-parse', 'HEAD')
            git('mv', 'old.go', 'docs/old.md')
            (root / 'docs/note.md').unlink()
            (root / 'frontend/line\nbreak.ts').write_text('export {}')
            git('add', '.')
            git('commit', '-qm', 'head')
            paths = policy.changed_paths(base, cwd=root)
            self.assertIn('old.go', paths)
            self.assertIn('frontend/line\nbreak.ts', paths)
            self.assertIn('docs/note.md', paths)
            self.assertEqual(policy.classify(paths), (True, True))

    def test_invalid_base_fails_closed(self):
        with self.assertRaises(subprocess.CalledProcessError):
            policy.changed_paths('invalid-base-that-does-not-exist')


if __name__ == '__main__':
    unittest.main()
