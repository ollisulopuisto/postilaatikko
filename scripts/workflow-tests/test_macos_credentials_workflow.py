"""Signing workflow guards, run with uv --no-project --with pyyaml."""
import importlib.util
import os
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

import yaml

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "credentials", ROOT / "scripts/macos_ci_credentials.py",
)
HELPER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HELPER)


class WorkflowTests(unittest.TestCase):
    def test_every_shell_parses_and_neither_route_passes_secrets_in_argv(self):
        jobs = yaml.safe_load((ROOT / ".github/workflows/build-macos.yml").read_text())["jobs"]
        for job in jobs.values():
            runs = "\n".join(s.get("run", "") for s in job["steps"])
            for secret in ['-p "$KEYCHAIN_PASSWORD"', '-P "$P12_PASSWORD"',
                           '--password "$APPLE_APP_PASSWORD"']:
                self.assertNotIn(secret, runs)
            self.assertIn("macos_ci_credentials.py import-certificate", runs)
            for step in job["steps"]:
                if "run" in step:
                    subprocess.run(["bash", "-n"], input=step["run"], text=True, check=True)
        runs = "\n".join(s.get("run", "") for s in jobs["draft-release"]["steps"])
        self.assertIn("--keychain-profile postilaatikko-ci-notary", runs)
        self.assertIn("shasum -a 256 -c SHA256.txt", runs)
        self.assertIn('basename "$DMG"', runs)
        for step in jobs["draft-release"]["steps"]:
            self.assertNotIn("actions/cache", step.get("uses", ""))
            self.assertNotIn("upload-artifact", step.get("uses", ""))

    def test_missing_certificate_cannot_skip_import_and_reach_notary_setup(self):
        jobs = yaml.safe_load((ROOT / ".github/workflows/build-macos.yml").read_text())["jobs"]
        for job in jobs.values():
            steps = job["steps"]
            index = next(i for i, step in enumerate(steps)
                         if step.get("name") == "Import Developer ID certificate")
            step = steps[index]
            self.assertNotIn("if", step)
            self.assertNotIn("continue-on-error", step)
            self.assertIn('test -n "$BUILD_CERTIFICATE_BASE64"', step["run"])
            self.assertIn('test -n "$P12_PASSWORD"', step["run"])
            self.assertIn('test -n "$KEYCHAIN_PASSWORD"', step["run"])
            for later in steps[index + 1:]:
                if "store-notary-credentials" in later.get("run", ""):
                    self.assertNotIn("if", later)
                    self.assertNotIn("continue-on-error", later)

    def test_security_import_transports_dummy_passwords_only_over_stdin(self):
        env = {"KEYCHAIN_PATH": "/dummy/keychain", "CERTIFICATE_PATH": "/dummy/cert.p12",
               "KEYCHAIN_PASSWORD": 'dummy "quote" \\ $value', "P12_PASSWORD": "dummy-p12"}
        with patch.dict(os.environ, env), patch.object(
            HELPER.subprocess, "run", return_value=subprocess.CompletedProcess([], 0),
        ) as run:
            HELPER.import_certificate()
        self.assertEqual(len(run.call_args_list), 8)
        for call in run.call_args_list:
            self.assertEqual(call.args[0], ["/usr/bin/security", "-i"])
            self.assertNotIn("dummy-p12", str(call.args))
            self.assertNotIn("KEYCHAIN_PASSWORD", call.kwargs["env"])
            self.assertNotIn("P12_PASSWORD", call.kwargs["env"])

    def test_zero_exit_with_command_error_fails_closed(self):
        with patch.object(HELPER.subprocess, "run", return_value=(
            subprocess.CompletedProcess([], 0, "", "unlock-keychain: returned -25293\n")
        )), self.assertRaises(RuntimeError):
            HELPER.security("unlock-keychain", "-p", "dummy", "/dummy/keychain")

    def test_import_preserves_existing_and_concurrent_keychains(self):
        env = {"KEYCHAIN_PATH": "/dummy/new", "CERTIFICATE_PATH": "/dummy/cert",
               "KEYCHAIN_PASSWORD": "dummy", "P12_PASSWORD": "dummy"}
        with patch.dict(os.environ, env), patch.object(HELPER, "security", side_effect=[
            '"/original with spaces"\n', "", "", "", "", "",
            '"/concurrent"\n', "",
        ]) as run:
            HELPER.import_certificate()
        self.assertEqual(run.call_args.args, ("list-keychains", "-d", "user", "-s",
                         "/original with spaces", "/concurrent", "/dummy/new"))

    def test_line_protocol_rejects_command_separators(self):
        for value in ["a\nb", "a\rb", "a\0b"]:
            with self.assertRaises(ValueError):
                HELPER.quote(value)

    def test_keychain_errors_never_echo_command_input(self):
        with patch.object(HELPER.subprocess, "run", return_value=(
            subprocess.CompletedProcess([], 1, "dummy-value", "dummy-value")
        )), self.assertRaises(RuntimeError) as error:
            HELPER.security("create-keychain", "-p", "dummy-value", "/dummy/path")
        self.assertNotIn("dummy-value", str(error.exception))


if __name__ == "__main__":
    unittest.main()
