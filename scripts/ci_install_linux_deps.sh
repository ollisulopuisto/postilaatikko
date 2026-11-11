#!/usr/bin/env bash
# Keep the standard Ubuntu runner, packages and signature verification.
set -euo pipefail

install_deps() {
  sudo timeout 150 apt-get -o Acquire::Retries=1 -o Acquire::http::Timeout=15 \
    -o Acquire::https::Timeout=15 -o APT::Update::Error-Mode=any "$@" update &&
  sudo timeout 300 apt-get -o Acquire::Retries=1 -o Acquire::http::Timeout=15 \
    -o Acquire::https::Timeout=15 "$@" install -y --no-install-recommends \
    libgtk-3-dev libwebkit2gtk-4.1-dev
}

if install_deps; then
  exit 0
fi

printf '%s\n' 'Runner mirror failed; retrying official Ubuntu archive directly.' >&2
# This fallback is scoped to these two apt calls; never rewrite the host sources.
. /etc/os-release
if [[ "$ID" != ubuntu || "$VERSION_CODENAME" != jammy || "$(dpkg --print-architecture)" != amd64 ]]; then
  printf '%s\n' 'Fallback supports only the existing Ubuntu 22.04 amd64 runner.' >&2
  exit 1
fi
sources=$(mktemp)
trap 'rm -f "$sources"' EXIT
cat > "$sources" <<SOURCES
deb https://archive.ubuntu.com/ubuntu jammy main restricted universe multiverse
deb https://archive.ubuntu.com/ubuntu jammy-updates main restricted universe multiverse
deb https://archive.ubuntu.com/ubuntu jammy-backports main restricted universe multiverse
deb https://security.ubuntu.com/ubuntu jammy-security main restricted universe multiverse
SOURCES
install_deps -o "Dir::Etc::sources.list=$sources" -o Dir::Etc::sourceparts=-
