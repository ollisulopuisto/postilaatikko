#!/usr/bin/env bash
# Manual only. Run in a disposable Ubuntu LTS VM with a working user keyring.
set -euo pipefail

if [[ ${1:-} == --help ]]; then
  cat <<'HELP'
Usage: LINUX_HELPER_SMOKE=1 scripts/linux-helper-smoke.sh /absolute/helper /absolute/new-profile existing-keyring-identity
Requires a disposable Ubuntu LTS VM, systemd user manager and unlocked keyring.
Initialize the named identity once yourself with helper init --identity NAME.
This installs/restarts/uninstalls the fixed user service and bootstraps a NEW DB.
The profile is retained for inspection. No accounts, sync, pairing, linger or reboot.
Do not use on a real account profile or alongside an existing helper service.
HELP
  exit 0
fi
if [[ ${LINUX_HELPER_SMOKE:-} != 1 || $# != 3 ]]; then
  printf '%s\n' 'Read --help, then explicitly set LINUX_HELPER_SMOKE=1 in a disposable VM.' >&2
  exit 2
fi
helper=$1
profile=$2
identity=$3
[[ $(uname -s) == Linux && $helper == /* && -x $helper && $profile == /* && ! -e $profile && ! -L $profile && -n $identity ]] || {
  printf '%s\n' 'Need Linux, an absolute executable, a new absolute profile and an existing identity name.' >&2
  exit 2
}
for command in systemctl curl python3; do
  command -v "$command" >/dev/stdout || exit 2
done
unit=${HOME:?}/.config/systemd/user/postilaatikko-helper.service
if [[ -e $unit || -L $unit ]]; then
  printf '%s\n' 'Existing service unit: refusing to change it.' >&2
  exit 2
fi
systemctl --user show-environment >/dev/stdout
# Refuse to collide with either listener before creating any state.
python3 - <<'PY'
import socket
for port in (8028, 8029):
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', port))
PY
"$helper" public-key --identity "$identity"
"$helper" bootstrap --data "$profile"
queue=$profile/smoke-queue
owns_unit=0
cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [[ $owns_unit == 1 && ( -e $unit || -L $unit ) ]]; then
    if ! "$helper" uninstall-agent; then
      printf '%s\n' 'Cleanup failed: inspect systemctl status and uninstall the smoke unit manually.' >&2
      status=1
    fi
  fi
  printf 'Retained smoke profile for inspection: %s\n' "$profile"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
owns_unit=1
"$helper" install-agent --identity "$identity" --data "$profile" --queue "$queue" --no-sync
"$helper" agent-status
systemctl --user restart postilaatikko-helper.service
ready=0
for _ in {1..20}; do
  if curl --fail --silent --show-error --max-time 2 http://127.0.0.1:8028/healthz; then
    ready=1
    break
  fi
  sleep 1
done
[[ $ready == 1 ]] || { printf '%s\n' 'Helper health did not become ready.' >&2; exit 1; }
"$helper" agent-status
"$helper" uninstall-agent
owns_unit=0
[[ ! -e $unit && ! -L $unit ]] || { printf '%s\n' 'Unit remains after uninstall.' >&2; exit 1; }
printf '%s\n' 'Smoke completed. This does not prove reboot/logout, mailbox sync, pairing or real-mail delivery.'
