#!/usr/bin/env sh
# bump-version.sh [major|minor|patch]
#
# Bumps MYCLOUD_VERSION in docker-compose.yml, commits the change, and
# creates a matching git tag.  Defaults to a patch bump when no argument
# is given.
#
# Usage:
#   ./scripts/bump-version.sh          # patch bump (1.2.4 -> 1.2.5)
#   ./scripts/bump-version.sh minor    # minor bump (1.2.4 -> 1.3.0)
#   ./scripts/bump-version.sh major    # major bump (1.2.4 -> 2.0.0)

set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/docker-compose.yml"

# ── Helpers ──────────────────────────────────────────────────────────────────

die() { echo "ERROR: $*" >&2; exit 1; }

# Extract the hardcoded fallback version from docker-compose.yml.
extract_version() {
  sed -n 's/.*MYCLOUD_VERSION:[[:space:]]*[$][{]MYCLOUD_VERSION:-\([^}]*\)[}].*/\1/p' \
    "$COMPOSE_FILE" | head -n 1
}

# Strip a leading 'v' or 'V'.
strip_v() { printf '%s' "$1" | sed 's/^[vV]//'; }

# ── Parse current version ─────────────────────────────────────────────────────

CURRENT="$(extract_version)"
[ -n "$CURRENT" ] || die "Could not read MYCLOUD_VERSION from $COMPOSE_FILE"

RAW="$(strip_v "$CURRENT")"
MAJOR="$(printf '%s' "$RAW" | cut -d. -f1)"
MINOR="$(printf '%s' "$RAW" | cut -d. -f2)"
PATCH="$(printf '%s' "$RAW" | cut -d. -f3)"

# Validate that all three components are numeric.
case "$MAJOR$MINOR$PATCH" in
  *[!0-9]*) die "Version '$CURRENT' does not look like vMAJOR.MINOR.PATCH" ;;
esac

# ── Bump ─────────────────────────────────────────────────────────────────────

BUMP="${1:-patch}"
case "$BUMP" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
  *) die "Unknown bump type '$BUMP'. Use: major | minor | patch" ;;
esac

NEW_VERSION="v${MAJOR}.${MINOR}.${PATCH}"

# ── Guard: clean working tree ─────────────────────────────────────────────────

cd "$REPO_ROOT"

if ! git diff --quiet || ! git diff --cached --quiet; then
  die "Working tree has uncommitted changes. Stash or commit them first."
fi

# ── Update docker-compose.yml ─────────────────────────────────────────────────

echo "Bumping $CURRENT -> $NEW_VERSION"

# Use | as the sed delimiter to avoid issues with forward slashes in paths.
sed -i "s|MYCLOUD_VERSION:-${CURRENT}|MYCLOUD_VERSION:-${NEW_VERSION}|" "$COMPOSE_FILE"

# Sanity-check: make sure the replacement landed.
AFTER="$(extract_version)"
[ "$AFTER" = "$NEW_VERSION" ] || \
  die "sed replacement failed — file still shows '$AFTER' instead of '$NEW_VERSION'"

# ── Commit and tag ────────────────────────────────────────────────────────────

git add "$COMPOSE_FILE"
git commit -m "chore: bump version to $NEW_VERSION"
git tag "$NEW_VERSION"

echo ""
echo "Version bumped to $NEW_VERSION."
echo "Run the following to publish:"
echo "  git push && git push --tags"
