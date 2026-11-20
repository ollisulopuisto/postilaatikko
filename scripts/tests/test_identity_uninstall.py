import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]

class IdentityUninstallTests(unittest.TestCase):
    def test_shell_uninstall_removes_own_dangling_alias(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            binary = home / '.local/bin'
            binary.mkdir(parents=True)
            (binary / 'postilaatikko').write_text('fixture')
            (binary / 'eterno-mail').symlink_to('postilaatikko')
            result = subprocess.run(['bash', str(ROOT / 'build/linux/uninstall.sh')],
                                    input='2\ny\ny\n', text=True, capture_output=True,
                                    env={**os.environ, 'HOME': str(home)})
            self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
            self.assertFalse((binary / 'postilaatikko').exists())
            self.assertFalse((binary / 'eterno-mail').is_symlink())

    def test_make_uninstall_only_removes_its_own_alias(self):
        for owned in [True, False]:
            with self.subTest(owned=owned), tempfile.TemporaryDirectory() as directory:
                binary = Path(directory) / 'usr/bin'
                binary.mkdir(parents=True)
                (binary / 'postilaatikko').write_text('fixture')
                alias = binary / 'eterno-mail'
                alias.symlink_to('postilaatikko' if owned else 'unrelated')
                result = subprocess.run(['make', 'uninstall-linux', 'DESTDIR=' + directory, 'PREFIX=/usr'],
                                        cwd=ROOT, text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(alias.is_symlink(), not owned)

if __name__ == '__main__':
    unittest.main()
