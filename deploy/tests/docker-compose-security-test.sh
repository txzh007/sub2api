#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

check_application_security_opt() {
  file=$1
  count=$(
    awk '
      $0 ~ /^  sub2api:([[:space:]]+&[A-Za-z0-9_-]+)?$/ {
        in_application = 1
        next
      }
      in_application && $0 ~ /^  [A-Za-z0-9_-]+:$/ {
        in_application = 0
      }
      in_application && $0 == "    security_opt:" {
        in_security_opt = 1
        next
      }
      in_application && in_security_opt && $0 == "      - no-new-privileges:true" {
        count++
      }
      END { print count + 0 }
    ' "$file"
  )

  if [ "$count" -ne 1 ]; then
    printf '%s must enable no-new-privileges exactly once for the sub2api service\n' "$file" >&2
    exit 1
  fi
}

check_ttoken_green_security_inheritance() {
  file=$1
  count=$(
    awk '
      $0 == "  ttoken-green:" {
        in_candidate = 1
        next
      }
      in_candidate && $0 ~ /^  [A-Za-z0-9_-]+:$/ {
        in_candidate = 0
      }
      in_candidate && $0 == "    <<: *sub2api-app" {
        count++
      }
      END { print count + 0 }
    ' "$file"
  )

  if [ "$count" -ne 1 ]; then
    printf '%s must inherit the secured sub2api application definition for ttoken-green\n' "$file" >&2
    exit 1
  fi
}

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml \
  deploy/docker-compose.dev.yml
do
  check_application_security_opt "$compose_file"
done

check_ttoken_green_security_inheritance deploy/docker-compose.yml

printf 'docker compose security test passed\n'
