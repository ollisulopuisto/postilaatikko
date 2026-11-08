"""Portable stapler retry tests. No Apple account, network or Mac required."""
import importlib.util
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("stapler", ROOT / "scripts/macos_stapler.py")
HELPER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HELPER)
TRANSIENT = ('Error Domain=NSURLErrorDomain Code=-1005 "The network connection was lost." '
             'https://api.apple-cloudkit.com/database/1/com.apple.gk.ticket-delivery/'
             'production/public/records/lookup')


def result(code, stderr="", stdout=""):
    return subprocess.CompletedProcess([], code, stdout, stderr)


class StaplerTests(unittest.TestCase):
    def invoke(self, results, action="validate"):
        with patch.object(HELPER.subprocess, "run", side_effect=results) as process, \
                patch.object(HELPER.time, "sleep") as sleep:
            code = HELPER.run(action, '/dummy/app "quoted" \\ name.app')
        return code, process, sleep

    def test_success_does_not_retry_and_target_is_one_argument(self):
        code, process, sleep = self.invoke([result(0)])
        self.assertEqual(code, 0)
        process.assert_called_once_with(
            ["xcrun", "stapler", "validate", '/dummy/app "quoted" \\ name.app'],
            capture_output=True, text=True, timeout=120, check=False,
        )
        sleep.assert_not_called()

    def test_observed_network_failure_can_recover_for_both_operations(self):
        for action in ("staple", "validate"):
            with self.subTest(action=action):
                code, process, sleep = self.invoke([
                    result(68, TRANSIENT), result(68, stdout=TRANSIENT), result(0),
                ], action)
                self.assertEqual(code, 0)
                self.assertEqual(process.call_count, 3)
                self.assertEqual([call.args[0] for call in sleep.call_args_list], [5, 15])

    def test_exhaustion_fails_closed_after_three_attempts(self):
        code, process, sleep = self.invoke([result(68, TRANSIENT)] * 3)
        self.assertEqual(code, 68)
        self.assertEqual(process.call_count, 3)
        self.assertEqual(sleep.call_count, 2)

    def test_other_ticket_signature_and_network_errors_fail_immediately(self):
        for failure in [result(65, "ticket missing"), result(68, "invalid ticket"),
                        result(68, "NSURLErrorDomain Code=-1005 other host"),
                        result(1, TRANSIENT),
                        result(68, TRANSIENT.replace("Code=-1005", "Code=-1001")),
                        result(68, TRANSIENT.replace("Code=-1005", "Code=-10050")),
                        result(-9, TRANSIENT)]:
            with self.subTest(failure=failure):
                code, process, sleep = self.invoke([failure])
                self.assertNotEqual(code, 0)
                process.assert_called_once()
                sleep.assert_not_called()

    def test_timeout_fails_without_retry(self):
        code, process, sleep = self.invoke([subprocess.TimeoutExpired("xcrun", 120)])
        self.assertEqual(code, 124)
        process.assert_called_once()
        sleep.assert_not_called()

    def test_invalid_action_never_launches_process(self):
        with patch.object(HELPER.subprocess, "run") as process, self.assertRaises(ValueError):
            HELPER.run("submit", "/dummy/app")
        process.assert_not_called()

    def test_workflow_keeps_app_and_dmg_staple_and_validation_mandatory(self):
        workflow = (ROOT / ".github/workflows/build-macos.yml").read_text()
        self.assertEqual(workflow.count("python3 scripts/macos_stapler.py staple "), 2)
        self.assertEqual(workflow.count("python3 scripts/macos_stapler.py validate "), 2)
        self.assertNotIn("xcrun stapler", workflow)
        for line in workflow.splitlines():
            if "scripts/macos_stapler.py" in line:
                self.assertNotIn("||", line)
        self.assertIn("codesign --verify", workflow)
        self.assertIn("hdiutil verify", workflow)


if __name__ == "__main__":
    unittest.main()
