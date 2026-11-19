import importlib.util
import unittest
from pathlib import Path


class FreshDMGVersionTest(unittest.TestCase):
    def test_used_base_is_rejected_for_final_and_test_tags(self):
        spec = importlib.util.spec_from_file_location('fresh_version', Path(__file__).with_name('check_new_dmg_version.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        for tag in ['v2026.11.19.1', 'v2026.11.19.1-test.123.1']:
            with self.subTest(tag=tag):
                self.assertFalse(module.is_fresh('2026.11.19.1', [tag]))
        self.assertTrue(module.is_fresh('2026.11.19.1', ['v2026.11.19.10', 'v2026.11.17.1-test.1.1']))
        self.assertTrue(module.is_fresh('2026.11.19.1', []))


if __name__ == '__main__':
    unittest.main()
