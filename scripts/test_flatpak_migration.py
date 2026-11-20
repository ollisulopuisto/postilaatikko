import importlib.util
from pathlib import Path
import tempfile
import unittest
spec = importlib.util.spec_from_file_location('migration', Path(__file__).with_name('migrate-flatpak-data.py'))
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)

class MigrationTests(unittest.TestCase):
    def test_copies_without_overwriting_and_retains_original(self):
        with tempfile.TemporaryDirectory() as temp:
            home=Path(temp);old=home/'.var/app'/migration.OLD;old.mkdir(parents=True)
            (old/'data').mkdir();(old/'data'/'aerion.db').write_bytes(b'mail-test')
            target=migration.migrate(home,set())
            self.assertEqual((target/'data'/'aerion.db').read_bytes(),b'mail-test')
            self.assertEqual((old/'data'/'aerion.db').read_bytes(),b'mail-test')
            with self.assertRaises(FileExistsError):migration.migrate(home,set())
    def test_refuses_running_and_symlinked_profiles(self):
        with tempfile.TemporaryDirectory() as temp:
            home=Path(temp);root=home/'.var/app';root.mkdir(parents=True)
            (root/migration.OLD).symlink_to(home,target_is_directory=True)
            with self.assertRaises(RuntimeError):migration.migrate(home,set())
            with self.assertRaises(RuntimeError):migration.migrate(home,{migration.OLD})
            self.assertFalse((root/migration.NEW).exists())
if __name__=='__main__':unittest.main()
