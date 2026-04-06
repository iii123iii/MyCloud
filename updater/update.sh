#!/usr/bin/env bash
set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────

PROJECT_DIR="${MYCLOUD_PROJECT_DIR:-/opt/mycloud}"
SERVICES="${MYCLOUD_UPDATE_SERVICES:-backend frontend nginx}"
TARGET_VERSION="${MYCLOUD_UPDATE_TARGET_VERSION:-}"
CURRENT_VERSION="${MYCLOUD_UPDATE_CURRENT_VERSION:-}"

cd "$PROJECT_DIR"

echo "── Pre-flight checks ──────────────────────────────────────"

if [ ! -d .git ]; then
  echo "ERROR: No git checkout at $PROJECT_DIR"
  exit 1
fi

if [ ! -S /var/run/docker.sock ]; then
  echo "ERROR: Docker socket not mounted"
  exit 1
fi

if [ ! -f docker-compose.yml ]; then
  echo "ERROR: docker-compose.yml not found at $PROJECT_DIR"
  exit 1
fi

# Abort if the working tree is dirty.
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "ERROR: Working tree has uncommitted changes at $PROJECT_DIR."
  echo "       Stash or revert them before running an update."
  exit 1
fi

# ── Pull latest code ─────────────────────────────────────────────────────────

echo ""
echo "── git pull ─────────────────────────────────────────────────"
git fetch --tags
git pull --ff-only

# ── Resolve version from git tags ─────────────────────────────────────────────
# Instead of reading Version.h (which requires a manual bump), derive the
# version from the latest reachable git tag.  This value is passed as a Docker
# build arg so the compiled binary embeds the correct version string.

GIT_VERSION="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [ -n "$GIT_VERSION" ]; then
  echo ""
  echo "Detected version from git tag: $GIT_VERSION"
  export MYCLOUD_VERSION="$GIT_VERSION"
fi

# ── Database migrations ──────────────────────────────────────────────────────

echo ""
echo "── Database migrations ────────────────────────────────────"

_db_host="${DB_HOST:-mariadb}"
_db_port="${DB_PORT:-3306}"
_db_name="${DB_NAME:-mycloud}"
_db_user="${DB_USER:-mycloud}"

# Resolve password: prefer Docker-secrets file, fall back to env var.
_db_pass=""
if [ -n "${DB_PASSWORD_FILE:-}" ] && [ -f "${DB_PASSWORD_FILE:-}" ]; then
  _db_pass="$(cat "$DB_PASSWORD_FILE")"
elif [ -n "${DB_PASSWORD:-}" ]; then
  _db_pass="$DB_PASSWORD"
fi

_found=0
for _sql in "$PROJECT_DIR"/backend/migrations/*.sql; do
  [ -f "$_sql" ] || continue
  _found=1
  echo "Applying migration: $(basename "$_sql")"
  if ! MYSQL_PWD="$_db_pass" mysql \
      -h "$_db_host" -P "$_db_port" \
      -u "$_db_user" "$_db_name" < "$_sql"; then
    echo "ERROR: migration $(basename "$_sql") failed."
    exit 1
  fi
done
[ "$_found" -eq 1 ] || echo "No migration files found — skipping."

# ── Build & restart ──────────────────────────────────────────────────────────
# The updater container is NOT in the services list, so it survives the restart.
# This is the key advantage over the old approach where the backend tried to
# rebuild itself.

echo ""
echo "── Building services: $SERVICES ─────────────────────────────"
# shellcheck disable=SC2086
docker compose build $SERVICES

echo ""
echo "── Restarting services: $SERVICES ───────────────────────────"
# shellcheck disable=SC2086
docker compose up -d $SERVICES

echo ""
echo "── Update complete ──────────────────────────────────────────"
echo "Services restarted: $SERVICES"
[ -n "${GIT_VERSION:-}" ] && echo "Version: $GIT_VERSION"
