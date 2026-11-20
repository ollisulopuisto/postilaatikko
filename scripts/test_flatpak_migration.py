import importlib.util
from pathlib import Path
import tempfile
import sqlite3
import unittest
spec = importlib.util.spec_from_file_location('migration', Path(__file__).with_name('migrate-flatpak-data.py'))
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)

class MigrationTests(unittest.TestCase):
    def test_copies_without_overwriting_and_retains_original(self):
        with tempfile.TemporaryDirectory() as temp:
            home=Path(temp);old=home/'.var/app'/migration.OLD;old.mkdir(parents=True)
            (old/'data').mkdir();(old/'data'/'cached-file').write_bytes(b'mail-test')
            target=migration.migrate(home,set())
            self.assertEqual((target/'data'/'cached-file').read_bytes(),b'mail-test')
            self.assertEqual((old/'data'/'cached-file').read_bytes(),b'mail-test')
            with self.assertRaises(FileExistsError):migration.migrate(home,set())
    def test_refuses_running_and_symlinked_profiles(self):
        with tempfile.TemporaryDirectory() as temp:
            home=Path(temp);root=home/'.var/app';root.mkdir(parents=True)
            (root/migration.OLD).symlink_to(home,target_is_directory=True)
            with self.assertRaises(RuntimeError):migration.migrate(home,set())
            with self.assertRaises(RuntimeError):migration.migrate(home,{migration.OLD})
            self.assertFalse((root/migration.NEW).exists())
    def test_rebases_only_copied_attachment_paths_in_destination_database(self):
        with tempfile.TemporaryDirectory() as temp:
            home=Path(temp);old=home/'.var/app'/migration.OLD;data=old/'data/aerion';data.mkdir(parents=True)
            cached=data/'attachments/a.pdf';cached.parent.mkdir();cached.write_bytes(b'offline-pdf')
            external=home/'Documents/personal.pdf'
            db=data/'aerion.db'
            with sqlite3.connect(db) as connection:
                connection.execute('CREATE TABLE attachments(id TEXT PRIMARY KEY, local_path TEXT)')
                connection.executemany('INSERT INTO attachments VALUES (?,?)',[('cached',str(cached)),('external',str(external))])
            target=migration.migrate(home,set())
            with sqlite3.connect(target/'data/aerion/aerion.db') as connection:
                paths=dict(connection.execute('SELECT id,local_path FROM attachments'))
            self.assertEqual(Path(paths['cached']).read_bytes(),b'offline-pdf')
            self.assertTrue(Path(paths['cached']).is_relative_to(target))
            self.assertEqual(paths['external'],str(external))
            with sqlite3.connect(db) as connection:
                self.assertEqual(connection.execute("SELECT local_path FROM attachments WHERE id='cached'").fetchone()[0],str(cached))
if __name__=='__main__':unittest.main()
