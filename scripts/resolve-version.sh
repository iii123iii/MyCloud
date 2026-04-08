#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PROJECT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"

if [ -n "${MYCLOUD_VERSION:-}" ]; then
  printf '%s\n' "$MYCLOUD_VERSION"
  exit 0
fi

cd "$PROJECT_DIR"

if ! command -v git >/dev/null 2>&1 || ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  printf 'dev\n'
  exit 0
fi

if version="$(git describe --tags --exact-match 2>/dev/null)"; then
  printf '%s\n' "$version"
  exit 0
fi

if version="$(git describe --tags --abbrev=0 2>/dev/null)"; then
  printf '%s\n' "$version"
  exit 0
fi

if sha="$(git rev-parse --short HEAD 2>/dev/null)"; then
  printf 'dev-%s\n' "$sha"
  exit 0
fi

printf 'dev\n'
