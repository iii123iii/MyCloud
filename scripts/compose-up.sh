#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PROJECT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"

if [ -z "${MYCLOUD_VERSION:-}" ]; then
  export MYCLOUD_VERSION="$(sh "$SCRIPT_DIR/resolve-version.sh")"
fi

echo "Using MYCLOUD_VERSION=$MYCLOUD_VERSION"

cd "$PROJECT_DIR"
exec docker compose "$@"
