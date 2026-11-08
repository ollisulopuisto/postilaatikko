#!/usr/bin/env python3
"""Retry only the observed Apple ticket-delivery connection-loss failure."""

import re
import subprocess
import sys
import time

DELAYS = (5, 15)


def ticket_connection_lost(result):
    output = (result.stdout or "") + (result.stderr or "")
    return (
        result.returncode == 68
        and re.search(r"NSURLErrorDomain\s+Code=-1005\b", output) is not None
        and "https://api.apple-cloudkit.com/database/1/com.apple.gk.ticket-delivery/" in output
    )


def run(action, target):
    if action not in ("staple", "validate"):
        raise ValueError("Only staple and validate are supported")
    for attempt in range(len(DELAYS) + 1):
        try:
            result = subprocess.run(
                ["xcrun", "stapler", action, target],
                capture_output=True, text=True, timeout=120, check=False,
            )
        except subprocess.TimeoutExpired:
            print("stapler exceeded 120 seconds; failing without retry", file=sys.stderr)
            return 124
        sys.stdout.write(result.stdout or "")
        sys.stderr.write(result.stderr or "")
        if result.returncode == 0:
            return 0
        if attempt == len(DELAYS) or not ticket_connection_lost(result):
            return result.returncode if result.returncode > 0 else 1
        delay = DELAYS[attempt]
        print(f"Apple ticket connection lost; retrying {action} in {delay}s "
              f"(attempt {attempt + 2}/3)", file=sys.stderr)
        time.sleep(delay)
    raise AssertionError("unreachable")


if __name__ == "__main__":
    if len(sys.argv) != 3 or sys.argv[1] not in ("staple", "validate"):
        raise SystemExit("Usage: macos_stapler.py {staple|validate} PATH")
    raise SystemExit(run(sys.argv[1], sys.argv[2]))
