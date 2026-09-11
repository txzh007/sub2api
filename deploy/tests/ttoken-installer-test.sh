#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd -P -- "$(dirname -- "$0")/../.." && pwd)"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "${TEST_ROOT}"' EXIT

mkdir -p "${TEST_ROOT}/bin" "${TEST_ROOT}/fixture" "${TEST_ROOT}/install" "${TEST_ROOT}/config"
printf '#!/bin/sh\necho test-ttoken-binary\n' > "${TEST_ROOT}/fixture/sub2api"
chmod +x "${TEST_ROOT}/fixture/sub2api"
cp "${REPO_ROOT}/deploy/ttoken.service" "${TEST_ROOT}/fixture/ttoken.service"
tar -C "${TEST_ROOT}/fixture" -czf "${TEST_ROOT}/fixture/ttoken_0.4.0_linux_amd64.tar.gz" sub2api ttoken.service
(
  cd "${TEST_ROOT}/fixture"
  sha256sum ttoken_0.4.0_linux_amd64.tar.gz > checksums.txt
)

cat > "${TEST_ROOT}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    http*) url="$1"; shift ;;
    *) shift ;;
  esac
done
case "${url}" in
  */ttoken_0.4.0_linux_amd64.tar.gz)
    cp "${TTOKEN_TEST_FIXTURE}/ttoken_0.4.0_linux_amd64.tar.gz" "${output}"
    ;;
  */checksums.txt)
    cp "${TTOKEN_TEST_FIXTURE}/checksums.txt" "${output}"
    ;;
  */releases\?per_page=100)
    printf '%s' '[{"tag_name":"v9.9.9"},{"tag_name":"ttoken-v0.3.0"},{"tag_name":"ttoken-v0.4.0"}]'
    ;;
  *)
    echo "unexpected curl URL: ${url}" >&2
    exit 1
    ;;
esac
EOF

cat > "${TEST_ROOT}/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${TTOKEN_TEST_SYSTEMCTL_LOG}"
if [[ "${1:-}" == "is-active" ]]; then
  exit 1
fi
EOF
chmod +x "${TEST_ROOT}/bin/curl" "${TEST_ROOT}/bin/systemctl"

export TTOKEN_TEST_FIXTURE="${TEST_ROOT}/fixture"
export TTOKEN_TEST_SYSTEMCTL_LOG="${TEST_ROOT}/systemctl.log"
export TTOKEN_INSTALL_DIR="${TEST_ROOT}/install"
export TTOKEN_CONFIG_DIR="${TEST_ROOT}/config"
export TTOKEN_SERVICE_NAME="ttoken-test"
export TTOKEN_SERVICE_USER="root"
export PATH="${TEST_ROOT}/bin:${PATH}"

bash "${REPO_ROOT}/deploy/install-ttoken.sh" install ttoken-v0.4.0
bash "${REPO_ROOT}/deploy/install-ttoken.sh" upgrade

test -x "${TEST_ROOT}/install/sub2api"
test "$("${TEST_ROOT}/install/sub2api")" = "test-ttoken-binary"
test -f /etc/systemd/system/ttoken-test.service
grep -q '^enable ttoken-test.service$' "${TEST_ROOT}/systemctl.log"
grep -q '^restart ttoken-test.service$' "${TEST_ROOT}/systemctl.log"

rm -f /etc/systemd/system/ttoken-test.service
echo "TToken installer test passed"
