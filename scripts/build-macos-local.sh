#!/usr/bin/env bash

set -Eeuo pipefail

readonly REQUIRED_GO_MINOR=26
readonly REQUIRED_NODE_MAJOR=24
readonly WAILS_VERSION=v2.15.0

usage() {
  cat <<'EOF'
Build Postilaatikko for the current Mac and optionally package a drag-and-drop DMG.

Usage:
  ./scripts/build-macos-local.sh [--app-only] [--skip-npm-ci]
  ./scripts/build-macos-local.sh --help

Options:
  --app-only     Build and sign the .app, but do not create a DMG.
  --skip-npm-ci  Reuse frontend/node_modules instead of reinstalling from the lockfile.
                 Use only when frontend dependencies have not changed.
  -h, --help     Show this help.

Artifacts:
  build/bin/postilaatikko.app
  build/bin/Postilaatikko-macos-arm64.dmg or -amd64.dmg

The DMG contains Postilaatikko.app and an /Applications shortcut. This is a local
build; it is not notarized and does not publish a release.
EOF
}

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "Required command not found: $1"
}

app_only=false
skip_npm_ci=false
while (($# > 0)); do
  case "$1" in
    --app-only)
      app_only=true
      ;;
    --skip-npm-ci)
      skip_npm_ci=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      fail "Unknown option: $1"
      ;;
  esac
  shift
done

[[ "$(uname -s)" == Darwin ]] || fail "This script builds macOS apps and must run on macOS."

for command_name in go node npm make codesign xcode-select xcrun; do
  require_command "$command_name"
done
if [[ "$app_only" == false ]]; then
  require_command hdiutil
fi

xcode-select -p >/dev/null 2>&1 || fail "Install Xcode Command Line Tools, then run: xcode-select --install"
xcrun --sdk macosx --show-sdk-path >/dev/null 2>&1 || fail "The macOS SDK is unavailable. Check your Xcode Command Line Tools installation."

if [[ -n "${GOROOT:-}" && ! -d "$GOROOT" ]]; then
  printf 'Warning: ignoring stale GOROOT=%s; Go will locate its toolchain automatically.\n' "$GOROOT" >&2
  unset GOROOT
fi

if ! go_version="$(go env GOVERSION 2>&1)"; then
  fail "Go could not start: $go_version. Check the Go installation and remove any invalid GOROOT override."
fi
if [[ ! "$go_version" =~ ^go([0-9]+)\.([0-9]+) ]]; then
  fail "Could not parse Go version '$go_version'; install Go 1.26 or newer."
fi
go_major="${BASH_REMATCH[1]}"
go_minor="${BASH_REMATCH[2]}"
if ((go_major < 1 || (go_major == 1 && go_minor < REQUIRED_GO_MINOR))); then
  fail "Go 1.26 or newer is required (found $go_version)."
fi

node_version="$(node --version)"
if [[ ! "$node_version" =~ ^v?([0-9]+)\. ]]; then
  fail "Could not parse Node.js version '$node_version'; install Node.js 24."
fi
node_major="${BASH_REMATCH[1]}"
if ((node_major != REQUIRED_NODE_MAJOR)); then
  fail "Node.js 24 is required to match the macOS build workflow (found $node_version)."
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/.." && pwd -P)"
cd "$repo_root"

case "$(uname -m)" in
  arm64|aarch64) artifact_arch=arm64 ;;
  x86_64|amd64) artifact_arch=amd64 ;;
  *) fail "Unsupported Mac architecture: $(uname -m)" ;;
esac

gopath="$(go env GOPATH)"
export PATH="$gopath/bin:$PATH"
printf 'Installing pinned Wails CLI %s if needed...\n' "$WAILS_VERSION"
env -u GOARCH -u GOOS go install "github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION}"

if [[ "$skip_npm_ci" == false ]]; then
  printf 'Installing frontend dependencies from package-lock.json...\n'
  npm ci --prefix "$repo_root/frontend"
fi

printf 'Building Postilaatikko for %s...\n' "$artifact_arch"
env -u GOARCH -u GOOS make build

app_path="$repo_root/build/bin/postilaatikko.app"
[[ -d "$app_path" ]] || fail "Build completed without creating $app_path"
codesign --verify --deep --strict "$app_path" || fail "The built app did not pass its ad-hoc signature check."

if [[ "$app_only" == true ]]; then
  printf '\nApp build ready:\n  %s\n' "$app_path"
  exit 0
fi

build_dir="$repo_root/build/bin"
mkdir -p "$build_dir"
staging_dir="$(mktemp -d "$build_dir/dmg-staging.XXXXXX")"
cleanup() {
  if [[ -n "${staging_dir:-}" && -d "$staging_dir" ]]; then
    rm -rf "$staging_dir"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cp -R "$app_path" "$staging_dir/Postilaatikko.app"
ln -s /Applications "$staging_dir/Applications"
dmg_path="$build_dir/Postilaatikko-macos-${artifact_arch}.dmg"

printf 'Creating drag-and-drop DMG...\n'
hdiutil create \
  -volname Postilaatikko \
  -srcfolder "$staging_dir" \
  -ov \
  -format UDZO \
  "$dmg_path"
hdiutil verify "$dmg_path"

printf '\nBuild ready:\n  %s\n  %s\n' "$app_path" "$dmg_path"
