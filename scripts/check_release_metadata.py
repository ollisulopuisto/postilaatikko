#!/usr/bin/env python3
"""Check package templates and release notes that feed the built app version."""
import json
import re
import sys
from pathlib import Path


def validate(root):
    version = (root / "VERSION").read_text().strip()
    issues = []
    for filename in ["Info.plist", "Info.dev.plist"]:
        text = (root / "build/darwin" / filename).read_text()
        for key in ["CFBundleVersion", "CFBundleShortVersionString"]:
            match = re.search(r"<key>" + key + r"</key>\s*<string>(.*?)</string>", text)
            if not match or match[1] != "{{.Info.ProductVersion}}":
                issues.append(f"{filename} {key} must use the Wails ProductVersion template")
    text = (root / "build/windows/info.json").read_text()
    for key in ["file_version", "ProductVersion"]:
        if f'"{key}": "{{{{.Info.ProductVersion}}}}"' not in text:
            issues.append(f"Windows {key} must use the Wails ProductVersion template")
    manifest = (root / "build/windows/wails.exe.manifest").read_text()
    if 'version="{{.Info.ProductVersion}}.0"' not in manifest:
        issues.append("Windows app manifest must use the Wails ProductVersion template")
    changelog = (root / "CHANGELOG.md").read_text()
    headings = re.findall(r"^## Postilaatikko v?(\S+)", changelog, re.M)
    if not headings or headings[0] != version:
        issues.append(f"Latest released changelog heading must match {version}")
    # Runtime About, CLI and What's New all receive the embedded VERSION value.
    main = (root / "main.go").read_text()
    if '//go:embed VERSION' not in main or 'app.Version = version' not in main:
        issues.append("Runtime version must be initialized from embedded VERSION")
    if json.loads((root / "wails.json").read_text())["info"]["productVersion"] != version:
        issues.append("Wails ProductVersion must match VERSION")
    return issues


if __name__ == "__main__":
    errors = validate(Path(__file__).resolve().parents[1])
    if errors:
        raise SystemExit("\n".join(errors))
    print("Package templates, runtime version and released What's New heading agree")
