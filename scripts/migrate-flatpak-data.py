#!/usr/bin/env python3
"""Copy an old Flatpak profile on the host, without replacing either profile."""
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys

OLD = 'io.github.wesleiaqui.eternomail'
NEW = 'io.github.ollisulopuisto.postilaatikko'


def migrate(home: Path, running: set[str]) -> Path:
    if OLD in running or NEW in running:
        raise RuntimeError('Quit both Flatpak applications before copying their data.')
    root = home / '.var' / 'app'
    source, target = root / OLD, root / NEW
    if source.is_symlink() or not source.is_dir():
        raise RuntimeError('Old profile is missing or is a symlink; inspect it manually. An EOL rebase may have already migrated it.')
    # mkdir is exclusive, including against concurrent migration invocations.
    # Do not remove a partial destination after an error: preserve all evidence.
    target.mkdir(mode=0o700)
    try:
        shutil.copytree(source, target, symlinks=True, dirs_exist_ok=True)
        target.chmod(0o700)
        rebase_attachments(source, target)
    except Exception as exc:
        raise RuntimeError(f'Copy failed. Old data is untouched. Inspect the partial destination at {target} before trying again.') from exc
    return target


def rebase_attachments(source: Path, target: Path) -> None:
    # Never follow copied links when locating writable databases. A SQLite
    # transaction changes only the destination's attachment references.
    for directory, _, files in os.walk(target, followlinks=False):
        if 'aerion.db' not in files:
            continue
        database = Path(directory) / 'aerion.db'
        if database.is_symlink():
            raise RuntimeError('Refusing a symlinked copied mailbox database.')
        with sqlite3.connect(database) as connection:
            if connection.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='attachments'").fetchone() is None:
                continue
            rows = connection.execute('SELECT id, local_path FROM attachments WHERE local_path IS NOT NULL').fetchall()
            for attachment_id, stored in rows:
                path = Path(stored)
                if not path.is_absolute() or '..' in path.parts:
                    continue
                try:
                    relative = path.relative_to(source)
                except ValueError:
                    continue  # User-selected external files keep their identity.
                relocated = target / relative
                connection.execute('UPDATE attachments SET local_path=? WHERE id=?', (str(relocated), attachment_id))


def main() -> int:
    try:
        result = subprocess.run(['flatpak', 'ps', '--columns=application'], check=True, capture_output=True, text=True)
        destination = migrate(Path.home(), set(result.stdout.split()))
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print(f'Copied profile to {destination}. Original retained. Review permissions and sign-ins before removing the old installation.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
