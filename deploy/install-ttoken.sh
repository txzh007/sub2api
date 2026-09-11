#!/usr/bin/env bash

set -euo pipefail

REPOSITORY="${TTOKEN_REPOSITORY:-txzh007/sub2api}"
INSTALL_DIR="${TTOKEN_INSTALL_DIR:-/opt/ttoken}"
CONFIG_DIR="${TTOKEN_CONFIG_DIR:-/etc/ttoken}"
SERVICE_NAME="${TTOKEN_SERVICE_NAME:-ttoken}"
SERVICE_USER="${TTOKEN_SERVICE_USER:-ttoken}"
ACTION="${1:-install}"
REQUESTED_TAG="${2:-}"

fail() {
  echo "TToken installer: $*" >&2
  exit 1
}

if [[ "${EUID}" -ne 0 ]]; then
  fail "run as root (for example: sudo bash install-ttoken.sh)"
fi

for command_name in curl grep sed sort tail tar sha256sum systemctl install id useradd cp; do
  command -v "${command_name}" >/dev/null 2>&1 || fail "missing required command: ${command_name}"
done

case "${ACTION}" in
  install|upgrade) ;;
  rollback)
    [[ -n "${REQUESTED_TAG}" ]] || fail "rollback requires ttoken-vX.Y.Z"
    ;;
  *) fail "usage: install-ttoken.sh [install|upgrade|rollback ttoken-vX.Y.Z]" ;;
esac

if [[ -z "${REQUESTED_TAG}" ]]; then
  # The public releases endpoint excludes drafts for unauthenticated requests.
  # Requiring the closing quote excludes prerelease suffixes such as -rc1.
  releases_json="$(curl -fsSL -H "Accept: application/vnd.github+json" "https://api.github.com/repos/${REPOSITORY}/releases?per_page=100")"
  REQUESTED_TAG="$(
    { printf '%s' "${releases_json}" |
      grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"ttoken-v[0-9]+\.[0-9]+\.[0-9]+"' |
      sed -E 's/.*"(ttoken-v[0-9]+\.[0-9]+\.[0-9]+)"/\1/' |
      sort -V |
      tail -n 1; } || true
  )"
fi

[[ "${REQUESTED_TAG}" =~ ^ttoken-v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "invalid TToken release tag: ${REQUESTED_TAG}"
VERSION="${REQUESTED_TAG#ttoken-v}"
ARCHIVE="ttoken_${VERSION}_linux_amd64.tar.gz"
RELEASE_BASE="https://github.com/${REPOSITORY}/releases/download/${REQUESTED_TAG}"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TEMP_DIR}"' EXIT

echo "Downloading TToken ${VERSION} for linux/amd64..."
curl -fsSL -o "${TEMP_DIR}/${ARCHIVE}" "${RELEASE_BASE}/${ARCHIVE}"
curl -fsSL -o "${TEMP_DIR}/checksums.txt" "${RELEASE_BASE}/checksums.txt"

(
  cd "${TEMP_DIR}"
  checksum_line="$(grep -E "^[[:xdigit:]]{64}[[:space:]]+${ARCHIVE}$" checksums.txt || true)"
  [[ -n "${checksum_line}" ]] || fail "checksum entry not found for ${ARCHIVE}"
  printf '%s\n' "${checksum_line}" | sha256sum -c -
  tar -xzf "${ARCHIVE}" sub2api ttoken.service
)

if ! id "${SERVICE_USER}" >/dev/null 2>&1; then
  useradd --system --home-dir "${INSTALL_DIR}" --shell /usr/sbin/nologin "${SERVICE_USER}"
fi
install -d -o "${SERVICE_USER}" -g "${SERVICE_USER}" -m 0750 "${INSTALL_DIR}" "${INSTALL_DIR}/data"
install -d -o root -g "${SERVICE_USER}" -m 0750 "${CONFIG_DIR}"

service_was_active=false
if systemctl is-active --quiet "${SERVICE_NAME}.service"; then
  service_was_active=true
  systemctl stop "${SERVICE_NAME}.service"
fi

if [[ -f "${INSTALL_DIR}/sub2api" ]]; then
  cp -a "${INSTALL_DIR}/sub2api" "${INSTALL_DIR}/sub2api.backup"
fi
install -o "${SERVICE_USER}" -g "${SERVICE_USER}" -m 0755 "${TEMP_DIR}/sub2api" "${INSTALL_DIR}/sub2api"

install -o root -g root -m 0644 "${TEMP_DIR}/ttoken.service" "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}.service" >/dev/null

if [[ "${ACTION}" == "install" || "${service_was_active}" == true ]]; then
  systemctl restart "${SERVICE_NAME}.service"
fi

echo "TToken ${VERSION} installed at ${INSTALL_DIR}/sub2api"
echo "Environment overrides: ${CONFIG_DIR}/ttoken.env"
echo "Service: systemctl status ${SERVICE_NAME}.service"
