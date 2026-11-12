#!/usr/bin/env python3
"""Guard: every *UsageDescription key in an Info.plist needs its hardened-runtime entitlement.

Compares the usage-description keys of an Info.plist (a rendered packaged plist
or the templated build/darwin/Info.plist) against an entitlements plist (the
repo's build/darwin/entitlements.plist or a `codesign -d --entitlements - --xml`
dump from a packaged app). Exits 1 when a usage string lacks its entitlement or
has no known entitlement mapping.
"""
import argparse
import plistlib
import re
import sys

USAGE_TO_ENTITLEMENT = {
    'NSContactsUsageDescription': 'com.apple.security.personal-information.addressbook',
    'NSCalendarsUsageDescription': 'com.apple.security.personal-information.calendars',
    'NSCalendarsFullAccessUsageDescription': 'com.apple.security.personal-information.calendars',
    'NSCameraUsageDescription': 'com.apple.security.device.camera',
    'NSMicrophoneUsageDescription': 'com.apple.security.device.audio-input',
    'NSAppleEventsUsageDescription': 'com.apple.security.automation.apple-events',
    'NSLocationUsageDescription': 'com.apple.security.personal-information.location',
    'NSLocationWhenInUseUsageDescription': 'com.apple.security.personal-information.location',
    'NSPhotoLibraryUsageDescription': 'com.apple.security.personal-information.photos-library',
}

USAGE_KEY_RE = re.compile(r'<key>\s*(NS[A-Za-z]*UsageDescription)\s*</key>')


def usage_keys(info_plist_path):
    """Usage-description keys from a rendered plist, falling back to a key scan
    so the templated build/darwin/Info.plist (Go template braces) still works."""
    raw = __import__('pathlib').Path(info_plist_path).read_bytes()
    try:
        parsed = plistlib.loads(raw)
    except Exception:
        parsed = None
    if isinstance(parsed, dict):
        return sorted(k for k in parsed if k.endswith('UsageDescription'))
    return sorted(set(USAGE_KEY_RE.findall(raw.decode('utf-8'))))


def entitlement_keys(entitlements_path):
    parsed = plistlib.loads(__import__('pathlib').Path(entitlements_path).read_bytes())
    if not isinstance(parsed, dict):
        raise ValueError(f'{entitlements_path}: not a plist dictionary')
    return {k for k, v in parsed.items() if v}


def missing_entitlements(info_plist_path, entitlements_path):
    """(usage_key, entitlement or None) pairs for usage strings without cover."""
    granted = entitlement_keys(entitlements_path)
    gaps = []
    for key in usage_keys(info_plist_path):
        entitlement = USAGE_TO_ENTITLEMENT.get(key)
        if entitlement is None or entitlement not in granted:
            gaps.append((key, entitlement))
    return gaps


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--info', required=True, help='Info.plist (packaged or templated)')
    parser.add_argument('--entitlements', required=True, help='entitlements plist or codesign --xml dump')
    args = parser.parse_args(argv)

    gaps = missing_entitlements(args.info, args.entitlements)
    for key, entitlement in gaps:
        if entitlement is None:
            print(f'{key}: no known entitlement mapping; extend USAGE_TO_ENTITLEMENT', file=sys.stderr)
        else:
            print(f'{key}: missing entitlement {entitlement}', file=sys.stderr)
    if gaps:
        return 1
    print(f'OK: every usage description in {args.info} has its entitlement')
    return 0


if __name__ == '__main__':
    sys.exit(main())
