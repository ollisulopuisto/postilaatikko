import importlib.util
import pathlib
import plistlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('check_entitlements', ROOT / 'scripts/check_entitlements.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

TEMPLATE_INFO = ROOT / 'build/darwin/Info.plist'


def write_plist(path, data):
    path.write_bytes(plistlib.dumps(data))


class CheckEntitlementsTests(unittest.TestCase):
    def test_template_info_plist_finds_contacts_usage_key(self):
        self.assertEqual(module.usage_keys(TEMPLATE_INFO), ['NSContactsUsageDescription'])

    def test_rendered_info_plist_parsed_with_plistlib(self):
        with tempfile.TemporaryDirectory() as directory:
            info = pathlib.Path(directory) / 'Info.plist'
            write_plist(info, {'CFBundleName': 'Postilaatikko', 'NSContactsUsageDescription': 'reads contacts'})
            self.assertEqual(module.usage_keys(info), ['NSContactsUsageDescription'])

    def test_missing_contacts_entitlement_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            ent = pathlib.Path(directory) / 'entitlements.plist'
            write_plist(ent, {})
            gaps = module.missing_entitlements(TEMPLATE_INFO, ent)
            self.assertEqual(
                gaps,
                [('NSContactsUsageDescription', 'com.apple.security.personal-information.addressbook')],
            )

    def test_contacts_entitlement_covers_the_usage_key(self):
        with tempfile.TemporaryDirectory() as directory:
            ent = pathlib.Path(directory) / 'entitlements.plist'
            write_plist(ent, {'com.apple.security.personal-information.addressbook': True})
            self.assertEqual(module.missing_entitlements(TEMPLATE_INFO, ent), [])

    def test_false_entitlement_value_does_not_count(self):
        with tempfile.TemporaryDirectory() as directory:
            ent = pathlib.Path(directory) / 'entitlements.plist'
            write_plist(ent, {'com.apple.security.personal-information.addressbook': False})
            gaps = module.missing_entitlements(TEMPLATE_INFO, ent)
            self.assertEqual(len(gaps), 1)

    def test_unknown_usage_key_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            info = pathlib.Path(directory) / 'Info.plist'
            ent = pathlib.Path(directory) / 'entitlements.plist'
            write_plist(info, {'NSWarpDriveUsageDescription': 'future key'})
            write_plist(ent, {'com.apple.security.personal-information.addressbook': True})
            self.assertEqual(module.missing_entitlements(info, ent), [('NSWarpDriveUsageDescription', None)])

    def test_cli_exit_codes(self):
        with tempfile.TemporaryDirectory() as directory:
            ent = pathlib.Path(directory) / 'entitlements.plist'
            write_plist(ent, {'com.apple.security.personal-information.addressbook': True})
            ok = subprocess.run(
                [sys.executable, str(ROOT / 'scripts/check_entitlements.py'),
                 '--info', str(TEMPLATE_INFO), '--entitlements', str(ent)],
                capture_output=True, text=True)
            self.assertEqual(ok.returncode, 0, ok.stderr)
            write_plist(ent, {})
            bad = subprocess.run(
                [sys.executable, str(ROOT / 'scripts/check_entitlements.py'),
                 '--info', str(TEMPLATE_INFO), '--entitlements', str(ent)],
                capture_output=True, text=True)
            self.assertEqual(bad.returncode, 1)
            self.assertIn('com.apple.security.personal-information.addressbook', bad.stderr)


if __name__ == '__main__':
    unittest.main()
