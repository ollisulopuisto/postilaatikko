#!/usr/bin/env python3
"""Refuse a DMG when the base version already appears in release history."""
import sys


def is_fresh(version, tags):
    final_tag = f'v{version}'
    return not any(tag == final_tag or tag.startswith(final_tag + '-test.') for tag in tags)


if __name__ == '__main__':
    if len(sys.argv) != 2:
        raise SystemExit('Usage: check_new_dmg_version.py VERSION < release-tags.txt')
    if not is_fresh(sys.argv[1], sys.stdin.read().splitlines()):
        raise SystemExit('This VERSION has already been packaged. Bump VERSION and its metadata before building another DMG.')
