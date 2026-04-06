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

# Validate each service name contains only safe characters (alphanumeric, hyphens, underscores).
# $SERVICES is intentionally word-split here — each token is one service name.
for _svc in $SERVICES; do
  case "$_svc" in
    *[!a-zA-Z0-9_-]*)
      echo "Invalid service name: $_svc (only alphanumeric, hyphens and underscores are allowed)"
      exit 1
      ;;
  esac
done

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

# Abort early if there are local modifications so git pull --ff-only doesn't fail
# mid-script with a cryptic error.
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "ERROR: Working tree has uncommitted changes at $PROJECT_DIR."
  echo "       Stash or revert them before running an update."
  exit 1
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

# Run any pending SQL migrations before restarting services so the new code
# always starts against an up-to-date schema.  All migration files use
# IF NOT EXISTS / IF EXISTS guards, so re-running them is safe.
run_migrations() {
  _db_host="${DB_HOST:-mariadb}"
  _db_port="${DB_PORT:-3306}"
  _db_name="${DB_NAME:-mycloud}"
  _db_user="${DB_USER:-mycloud}"

  # Resolve password: prefer the Docker-secrets file, fall back to env var.
  _db_pass=""
  if [ -n "${DB_PASSWORD_FILE:-}" ] && [ -f "$DB_PASSWORD_FILE" ]; then
    _db_pass="$(cat "$DB_PASSWORD_FILE")"
  elif [ -n "${DB_PASSWORD:-}" ]; then
    _db_pass="$DB_PASSWORD"
  fi

  _found=0
  for _sql in "$PROJECT_DIR"/backend/migrations/*.sql; do
    [ -f "$_sql" ] || continue
    _found=1
    echo "Applying migration: $(basename "$_sql")"
    # Pass the password via MYSQL_PWD so it does not appear in the process list.
    if ! MYSQL_PWD="$_db_pass" mysql \
        -h "$_db_host" -P "$_db_port" \
        -u "$_db_user" "$_db_name" < "$_sql"; then
      echo "ERROR: migration $(basename "$_sql") failed."
      return 1
    fi
  done

  [ "$_found" -eq 1 ] || echo "No migration files found — skipping."
}

echo "Running database migrations..."
run_migrations

# Rebuild and restart the web-facing services after updating the checkout.
# shellcheck disable=SC2086  -- word-split of $SERVICES is intentional (validated above)
set -- $SERVICES
docker compose up -d --build "$@"
