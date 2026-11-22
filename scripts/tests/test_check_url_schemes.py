import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('check_url_schemes', ROOT / 'scripts/check_url_schemes.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

SCRIPT = ROOT / 'scripts/check_url_schemes.py'
WAILS_JSON = ROOT / 'wails.json'

PLIST_TEMPLATE = '''<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleURLTypes</key>
<array>
{entries}
</array>
</dict></plist>
'''
PLIST_ENTRY = '''<dict>
<key>CFBundleURLName</key><string>com.wails.{scheme}</string>
<key>CFBundleURLSchemes</key><array><string>{scheme}</string></array>
<key>CFBundleTypeRole</key><string>{role}</string>
</dict>'''


def doc(protocols):
    return {'info': {'protocols': protocols}}


def plist(*entries):
    body = '\n'.join(PLIST_ENTRY.format(scheme=s, role=r) for s, r in entries)
    return PLIST_TEMPLATE.format(entries=body).encode()


class WailsJsonModeTests(unittest.TestCase):
    def test_repo_wails_json_passes(self):
        result = subprocess.run(
            [sys.executable, str(SCRIPT), str(WAILS_JSON)],
            capture_output=True, text=True, cwd=ROOT,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_valid_protocols_have_no_problems(self):
        problems = module.protocol_problems(doc([
            {'scheme': 'mailto', 'description': 'Email', 'role': 'Editor'},
            {'scheme': 'webcal', 'role': 'Viewer'},
        ]))
        self.assertEqual(problems, [])

    def test_missing_role_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'mailto', 'description': 'Email'}]))
        self.assertTrue(any('mailto' in p and 'role' in p for p in problems), problems)

    def test_invalid_role_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'mailto', 'role': 'editor'}]))
        self.assertTrue(any("'editor'" in p for p in problems), problems)

    def test_mailto_viewer_role_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'mailto', 'role': 'Viewer'}]))
        self.assertTrue(any('Editor' in p for p in problems), problems)

    def test_duplicate_mailto_is_reported(self):
        problems = module.protocol_problems(doc([
            {'scheme': 'mailto', 'role': 'Editor'},
            {'scheme': 'mailto', 'role': 'Viewer'},
        ]))
        self.assertTrue(any('2 times' in p for p in problems), problems)

    def test_missing_scheme_is_reported(self):
        problems = module.protocol_problems(doc([{'role': 'Editor'}]))
        joined = '\n'.join(problems)
        self.assertIn('no scheme', joined)
        self.assertIn('mailto', joined)

    def test_missing_protocols_block_is_reported(self):
        self.assertEqual(len(module.protocol_problems({'info': {}})), 1)

    def test_absent_mailto_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'webcal', 'role': 'Viewer'}]))
        self.assertEqual(problems, ['mailto is not registered'])

    def test_cli_rejects_bad_file(self):
        with tempfile.TemporaryDirectory() as directory:
            bad = pathlib.Path(directory) / 'wails.json'
            bad.write_text(json.dumps(doc([{'scheme': 'mailto'}])), encoding='utf-8')
            result = subprocess.run(
                [sys.executable, str(SCRIPT), str(bad)],
                capture_output=True, text=True,
            )
            self.assertEqual(result.returncode, 1)
            self.assertIn('role', result.stderr)


class PlistModeTests(unittest.TestCase):
    def run_cli(self, raw):
        with tempfile.TemporaryDirectory() as directory:
            plist_path = pathlib.Path(directory) / 'Info.plist'
            plist_path.write_bytes(raw)
            return subprocess.run(
                [sys.executable, str(SCRIPT), str(plist_path)],
                capture_output=True, text=True,
            )

    def test_valid_packaged_plist_passes(self):
        problems = module.plist_problems(plist(('mailto', 'Editor')))
        self.assertEqual(problems, [])

    def test_empty_role_is_reported(self):
        problems = module.plist_problems(plist(('mailto', '')))
        self.assertTrue(any('role' in p for p in problems), problems)

    def test_missing_urltypes_block_is_reported(self):
        raw = b'<?xml version="1.0"?><plist version="1.0"><dict></dict></plist>'
        self.assertEqual(module.plist_problems(raw), ['CFBundleURLTypes is missing'])

    def test_missing_mailto_is_reported(self):
        problems = module.plist_problems(plist(('webcal', 'Viewer')))
        self.assertIn('mailto is not registered', problems)

    def test_duplicate_scheme_is_reported(self):
        problems = module.plist_problems(plist(('mailto', 'Editor'), ('mailto', 'Viewer')))
        self.assertTrue(any('2 times' in p for p in problems), problems)

    def test_cli_accepts_wails_rendered_plist_without_xml_declaration(self):
        # The Wails template omits the <?xml?> declaration.
        raw = plist(('mailto', 'Editor')).split(b'?>', 1)[1]
        result = self.run_cli(raw)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_cli_rejects_bad_plist(self):
        result = self.run_cli(plist(('mailto', '')))
        self.assertEqual(result.returncode, 1)
        self.assertIn('role', result.stderr)


if __name__ == '__main__':
    unittest.main()
