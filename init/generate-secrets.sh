#!/bin/sh
set -e

DIR="/run/secrets"
mkdir -p "$DIR"

gen() {
  file="$DIR/$1"
  bytes="$2"
  if [ ! -f "$file" ] || [ ! -s "$file" ]; then
    openssl rand -hex "$bytes" > "$file"
    echo "[init] Generated $1"
  else
    echo "[init] $1 already exists - skipping"
  fi
}

# 64 bytes = 128-char hex JWT secret
gen jwt_secret 64

# 32 bytes = 64-char hex AES-256 master encryption key
gen master_encryption_key 32

# 16 bytes = 32-char hex DB passwords
gen db_password 16
gen db_root_password 16

echo "[init] All secrets ready."
