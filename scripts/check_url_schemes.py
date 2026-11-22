#!/usr/bin/env python3
"""Guard: every URL protocol in wails.json must stay registrable on macOS.

Wails renders build/darwin/Info.plist from the info.protocols entries in
wails.json. macOS LaunchServices registers an app as a handler for a URL
scheme only when the scheme's CFBundleURLTypes entry carries a valid
CFBundleTypeRole (Editor/Viewer/Shell/None); Apple documents the key as
required, and the Wails plist template emits it empty when the protocol
entry has no role. A mail client whose mailto entry has no role does not
appear among the system's mail apps.

Exits 1 when any protocol entry lacks a scheme or a valid role, or when
mailto is not registered at all.
"""
import json
import sys

VALID_ROLES = ('Editor', 'Viewer', 'Shell', 'None')
REQUIRED_SCHEME = 'mailto'


def protocol_problems(doc):
    info = doc.get('info') or {}
    protocols = info.get('protocols')
    if not protocols:
        return [f'info.protocols is missing or empty; {REQUIRED_SCHEME} is not registered']
    problems = []
    schemes = []
    for entry in protocols:
        if not isinstance(entry, dict):
            problems.append(f'protocol entry {entry!r} is not an object')
            continue
        scheme = str(entry.get('scheme') or '').strip()
        role = str(entry.get('role') or '').strip()
        if not scheme:
            problems.append('a protocol entry has no scheme')
        else:
            schemes.append(scheme)
        if role not in VALID_ROLES:
            problems.append(
                f'{scheme or "<no scheme>"}: role {role!r} must be one of {VALID_ROLES} '
                '(CFBundleTypeRole is required for LaunchServices registration)'
            )
    if REQUIRED_SCHEME not in schemes:
        problems.append(f'{REQUIRED_SCHEME} is not listed in info.protocols')
    return problems


if __name__ == '__main__':
    path = sys.argv[1] if len(sys.argv) > 1 else 'wails.json'
    with open(path, encoding='utf-8') as handle:
        problems = protocol_problems(json.load(handle))
    if problems:
        for problem in problems:
            print(f'{path}: {problem}', file=sys.stderr)
        raise SystemExit(1)
    print(f'{path}: URL protocols registrable on macOS')
