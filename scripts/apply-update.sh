#!/usr/bin/env sh
set -eu

PROJECT_DIR="${MYCLOUD_PROJECT_DIR:-/opt/mycloud}"
SERVICES="${MYCLOUD_UPDATE_SERVICES:-backend frontend nginx}"
TARGET_VERSION="${MYCLOUD_UPDATE_TARGET_VERSION:-}"
CURRENT_VERSION="${MYCLOUD_UPDATE_CURRENT_VERSION:-}"

cd "$PROJECT_DIR"

if [ ! -d .git ]; then
  echo "Expected a git checkout at $PROJECT_DIR"
  exit 1
fi

if [ ! -S /var/run/docker.sock ]; then
  echo "Docker socket is not mounted at /var/run/docker.sock"
  exit 1
fi

normalize_version() {
  printf '%s' "$1" | sed 's/^[vV]//'
}

version_is_greater() {
  left="$(normalize_version "$1")"
  right="$(normalize_version "$2")"

  if [ "$left" = "$right" ]; then
    return 1
  fi

  highest="$(printf '%s\n%s\n' "$left" "$right" | sort -V | tail -n 1)"
  [ "$highest" = "$left" ]
}

extract_checkout_version() {
  sed -n 's/^#define MYCLOUD_VERSION[[:space:]]*"\([^"]*\)".*/\1/p' \
    backend/src/utils/Version.h | head -n 1
}

if [ -n "$TARGET_VERSION" ] && [ -n "$CURRENT_VERSION" ]; then
  if ! version_is_greater "$TARGET_VERSION" "$CURRENT_VERSION"; then
    echo "Target version $TARGET_VERSION is not newer than current version $CURRENT_VERSION."
    exit 0
  fi
fi

git pull --ff-only

CHECKED_OUT_VERSION="$(extract_checkout_version || true)"
if [ -n "$TARGET_VERSION" ] && [ -n "$CURRENT_VERSION" ] && [ -n "$CHECKED_OUT_VERSION" ]; then
  if [ "$CHECKED_OUT_VERSION" != "$TARGET_VERSION" ] && \
     ! version_is_greater "$CHECKED_OUT_VERSION" "$CURRENT_VERSION"; then
    echo "Checked out version $CHECKED_OUT_VERSION did not advance to target $TARGET_VERSION."
    exit 1
  fi
fi

# Rebuild and restart the web-facing services after updating the checkout.
docker compose up -d --build $SERVICES
