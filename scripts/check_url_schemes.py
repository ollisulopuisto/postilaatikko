#!/usr/bin/env python3
"""Guard: every URL protocol must stay registrable on macOS.

Wails renders build/darwin/Info.plist from the info.protocols entries in
wails.json. macOS LaunchServices registers an app as a handler for a URL
scheme only when the scheme's CFBundleURLTypes entry carries a valid
CFBundleTypeRole (Editor/Viewer/Shell/None); Apple documents the key as
required, and the Wails plist template emits it empty when the protocol
entry has no role. A mail client whose mailto entry has no role does not
appear among the system's mail apps.

Two modes:
  wails.json mode (default): validate the info.protocols source entries.
  plist mode (path not ending in .json): validate a rendered or packaged
  Info.plist (regex fallback, because the Wails template omits the XML
  declaration that plistlib requires).

Both modes require exactly one mailto registration with role "Editor"
(this is a mail client; composing is editing) and no duplicate schemes.
Exits 1 on any problem.
"""
import json
import re
import sys

VALID_ROLES = ('Editor', 'Viewer', 'Shell', 'None')
REQUIRED_SCHEME = 'mailto'
REQUIRED_ROLE = 'Editor'

URLTYPES_BLOCK_RE = re.compile(
    r'<key>\s*CFBundleURLTypes\s*</key>\s*<array>((?:\s*<dict>.*?</dict>)*)\s*</array>',
    re.S)
URLTYPE_DICT_RE = re.compile(r'<dict>(.*?)</dict>', re.S)
ROLE_RE = re.compile(
    r'<key>\s*CFBundleTypeRole\s*</key>\s*<string>\s*(.*?)\s*</string>', re.S)
SCHEMES_BLOCK_RE = re.compile(
    r'<key>\s*CFBundleURLSchemes\s*</key>\s*<array>(.*?)</array>', re.S)
STRING_RE = re.compile(r'<string>\s*(.*?)\s*</string>')


def shared_problems(entries):
    """entries: list of (scheme-list, role) pairs."""
    problems = []
    seen = {}
    for schemes, role in entries:
        if role not in VALID_ROLES:
            problems.append(
                f'role {role!r} must be one of {VALID_ROLES} '
                '(CFBundleTypeRole is required for LaunchServices registration)'
            )
        for scheme in schemes:
            seen.setdefault(scheme, []).append(role)
    for scheme, roles in seen.items():
        if len(roles) > 1:
            problems.append(f'{scheme}: registered {len(roles)} times; exactly once is required')
    mailto_roles = seen.get(REQUIRED_SCHEME, [])
    if not mailto_roles:
        problems.append(f'{REQUIRED_SCHEME} is not registered')
    elif mailto_roles[0] != REQUIRED_ROLE:
        problems.append(
            f'{REQUIRED_SCHEME}: role {mailto_roles[0]!r} must be {REQUIRED_ROLE!r} '
            '(a mail client composes messages for mailto URLs)'
        )
    return problems


def protocol_problems(doc):
    info = doc.get('info') or {}
    protocols = info.get('protocols')
    if not protocols:
        return [f'info.protocols is missing or empty; {REQUIRED_SCHEME} is not registered']
    problems = []
    entries = []
    for entry in protocols:
        if not isinstance(entry, dict):
            problems.append(f'protocol entry {entry!r} is not an object')
            continue
        scheme = str(entry.get('scheme') or '').strip()
        role = str(entry.get('role') or '').strip()
        if not scheme:
            problems.append('a protocol entry has no scheme')
        entries.append(([scheme] if scheme else [], role))
    return problems + shared_problems(entries)


def plist_problems(raw):
    text = raw.decode('utf-8', errors='replace')
    block = URLTYPES_BLOCK_RE.search(text)
    if not block:
        return ['CFBundleURLTypes is missing']
    entries = []
    problems = []
    for body in URLTYPE_DICT_RE.findall(block.group(1)):
        role_match = ROLE_RE.search(body)
        role = role_match.group(1) if role_match else ''
        schemes_block = SCHEMES_BLOCK_RE.search(body)
        schemes = STRING_RE.findall(schemes_block.group(1)) if schemes_block else []
        if not schemes:
            problems.append('a CFBundleURLTypes entry has no CFBundleURLSchemes')
        entries.append(([s.strip() for s in schemes], role))
    return problems + shared_problems(entries)


if __name__ == '__main__':
    path = sys.argv[1] if len(sys.argv) > 1 else 'wails.json'
    if path.endswith('.json'):
        with open(path, encoding='utf-8') as handle:
            problems = protocol_problems(json.load(handle))
    else:
        with open(path, 'rb') as handle:
            problems = plist_problems(handle.read())
    if problems:
        for problem in problems:
            print(f'{path}: {problem}', file=sys.stderr)
        raise SystemExit(1)
    print(f'{path}: URL protocols registrable on macOS')
