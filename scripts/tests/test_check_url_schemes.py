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


def doc(protocols):
    return {'info': {'protocols': protocols}}


class CheckUrlSchemesTests(unittest.TestCase):
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
        self.assertEqual(len(problems), 1)
        self.assertIn('mailto', problems[0])
        self.assertIn('role', problems[0])

    def test_empty_role_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'mailto', 'role': ''}]))
        self.assertEqual(len(problems), 1)
        self.assertIn('mailto', problems[0])

    def test_invalid_role_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'mailto', 'role': 'editor'}]))
        self.assertEqual(len(problems), 1)
        self.assertIn("'editor'", problems[0])

    def test_missing_scheme_is_reported(self):
        problems = module.protocol_problems(doc([{'role': 'Editor'}]))
        joined = '\n'.join(problems)
        self.assertIn('no scheme', joined)
        self.assertIn('mailto', joined)

    def test_missing_protocols_block_is_reported(self):
        self.assertEqual(len(module.protocol_problems({'info': {}})), 1)

    def test_absent_mailto_is_reported(self):
        problems = module.protocol_problems(doc([{'scheme': 'webcal', 'role': 'Viewer'}]))
        self.assertEqual(len(problems), 1)
        self.assertIn('mailto', problems[0])

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


if __name__ == '__main__':
    unittest.main()
