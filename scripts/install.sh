#!/usr/bin/env bash
# Hardened host preflight and local-artifact installer.
# Remote release discovery and download are intentionally unavailable.

set -Eeuo pipefail

die() {
  echo "vocat-install: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  scripts/install.sh --check-env
  sudo scripts/install.sh --artifact /path/to/dist/hardened/<commit> \
    --expected-commit <40-hex-sha> \
    --expected-index-sha256 <sha256-of-SHA256SUMS>

The hardened installer never downloads a release or selects a latest version.
Build a committed revision with scripts/build-hardened.sh, transfer the complete
artifact directory, then deploy it with --artifact.
EOF
}

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
deploy_script="$script_dir/deploy-hardened.sh"

check_environment() {
  local failed=0
  local command
  for command in curl ip jq qmicli sha256sum sqlite3 ss systemctl; do
    if ! command -v "$command" >/dev/null 2>&1; then
      echo "missing required command: $command" >&2
      failed=1
    fi
  done

  if ! command -v qmi-proxy >/dev/null 2>&1 &&
    [ ! -x /usr/libexec/qmi-proxy ] &&
    [ ! -x /usr/lib/qmi-proxy ] &&
    [ ! -x /usr/lib/libqmi-glib/qmi-proxy ]; then
    echo "missing required QMI proxy: qmi-proxy" >&2
    failed=1
  fi

  if command -v ip >/dev/null 2>&1 && ! ip xfrm state list >/dev/null 2>&1; then
    echo "XFRM/IPsec is unavailable or cannot be queried; run this check as root" >&2
    failed=1
  fi
  if ! getent group vocat-modem >/dev/null 2>&1; then
    echo "missing device-access group: vocat-modem" >&2
    failed=1
  fi
  if ! getent passwd vocat >/dev/null 2>&1; then
    echo "missing service account: vocat" >&2
    failed=1
  fi

  if [ "$failed" -ne 0 ]; then
    die "environment preflight failed; use the reviewed guest preparation script"
  fi
  echo "VoCat hardened environment preflight passed"
}

case "${1:-}" in
  --check-env)
    [ "$#" -eq 1 ] || die "--check-env does not accept additional arguments"
    check_environment
    ;;
  --artifact)
    [ "$#" -eq 6 ] && [ "$3" = "--expected-commit" ] && [ "$5" = "--expected-index-sha256" ] || \
      die "--artifact requires a directory, --expected-commit, and --expected-index-sha256"
    [ "$(id -u)" -eq 0 ] || die "artifact deployment must run as root"
    [ -x "$deploy_script" ] || die "missing hardened deploy script: $deploy_script"
    exec "$deploy_script" --expected-commit "$4" --expected-index-sha256 "$6" "$2"
    ;;
  -h|--help)
    usage
    ;;
  "")
    usage >&2
    exit 2
    ;;
  *)
    die "remote installation and self-update are disabled; use --artifact"
    ;;
esac
