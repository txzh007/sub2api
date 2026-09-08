#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
BACKEND_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="$(CDPATH= cd -- "$BACKEND_DIR/.." && pwd)"
VERSION_FILE="$BACKEND_DIR/cmd/server/VERSION"
TTOKEN_VERSION_FILE="$BACKEND_DIR/cmd/server/TTOKEN_VERSION"

# TToken release tags remain independent from the upstream VERSION file.
if command -v git >/dev/null 2>&1; then
  TAG="$(
    git -C "$REPO_DIR" describe --tags --exact-match --match 'ttoken-v[0-9]*' 2>/dev/null || \
    git -C "$REPO_DIR" describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null || \
    git -C "$REPO_DIR" describe --tags --exact-match --match '[0-9]*' 2>/dev/null || \
    true
  )"
  if [ -n "$TAG" ]; then
    TAG="${TAG#ttoken-v}"
    printf '%s\n' "${TAG#v}"
    exit 0
  fi
fi

if [ -f "$TTOKEN_VERSION_FILE" ]; then
  printf '%s\n' "$(tr -d '\r\n' < "$TTOKEN_VERSION_FILE")"
  exit 0
fi

# Compatibility fallback for a source tree without TToken metadata.
printf '%s\n' "$(tr -d '\r\n' < "$VERSION_FILE")"
