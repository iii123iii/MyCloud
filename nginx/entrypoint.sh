#!/bin/sh
set -e

SSL_DIR="/etc/nginx/ssl"
CERT="$SSL_DIR/server.crt"
KEY="$SSL_DIR/server.key"

mkdir -p "$SSL_DIR"

if [ ! -f "$CERT" ] || [ ! -f "$KEY" ]; then
  echo "[nginx] Generating self-signed TLS certificate (valid 10 years)..."
  openssl req -x509 -nodes -days 3650 \
    -newkey rsa:4096 \
    -keyout "$KEY" \
    -out "$CERT" \
    -subj "/C=US/ST=Local/L=Local/O=MyCloud/OU=MyCloud/CN=localhost" \
    -addext "subjectAltName=IP:127.0.0.1,DNS:localhost"
  echo "[nginx] Certificate generated at $CERT"
else
  echo "[nginx] Existing TLS certificate found, skipping generation."
fi

echo "[nginx] Starting Nginx..."
exec nginx -g "daemon off;"
