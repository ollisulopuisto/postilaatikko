import importlib.util
import shutil
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("release_metadata", ROOT / "scripts/check_release_metadata.py")
METADATA = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(METADATA)
FILES = ["VERSION", "CHANGELOG.md", "main.go", "wails.json", "build/darwin/Info.plist",
         "build/darwin/Info.dev.plist", "build/windows/info.json", "build/windows/wails.exe.manifest"]


class ReleaseMetadataTests(unittest.TestCase):
    def test_current_tree_passes(self):
        self.assertEqual(METADATA.validate(ROOT), [])

    def test_each_literal_plist_version_and_changelog_drift_is_rejected(self):
        cases = [
            ("build/darwin/Info.plist", "{{.Info.ProductVersion}}", "old.literal", 1),
            ("build/darwin/Info.dev.plist", "{{.Info.ProductVersion}}", "old.literal", 1),
            ("build/windows/info.json", "{{.Info.ProductVersion}}", "old.literal", 1),
            ("build/windows/wails.exe.manifest", "{{.Info.ProductVersion}}", "old.literal", 1),
            ("CHANGELOG.md", "## Postilaatikko " + (ROOT / "VERSION").read_text().strip(),
             "## Postilaatikko 2000.01.01", 1),
        ]
        for filename, old, new, count in cases:
            with self.subTest(filename=filename), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                for item in FILES:
                    target = root / item
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(ROOT / item, target)
                target = root / filename
                target.write_text(target.read_text().replace(old, new, count))
                self.assertTrue(METADATA.validate(root))


if __name__ == "__main__":
    unittest.main()
