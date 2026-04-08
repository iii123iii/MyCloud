FROM golang:1.26.1-bookworm AS builder

ARG MYCLOUD_VERSION

WORKDIR /src
COPY backend-go/go.mod backend-go/go.sum ./backend-go/
WORKDIR /src/backend-go
RUN go mod download

WORKDIR /src
COPY . .

RUN set -eu; \
    version="${MYCLOUD_VERSION:-}"; \
    if [ -z "$version" ]; then \
      if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
        if version="$(git describe --tags --dirty --always 2>/dev/null)"; then \
          :; \
        else \
          sha="$(git rev-parse --short HEAD 2>/dev/null || true)"; \
          if [ -n "$sha" ]; then version="dev-$sha"; else version="dev"; fi; \
        fi; \
      else \
        version="dev"; \
      fi; \
    fi; \
    printf '%s' "$version" >/tmp/mycloud-version

WORKDIR /src/backend-go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/mycloud_backend ./cmd/api

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

ENV MYCLOUD_VERSION_FILE=/etc/mycloud/version

COPY --from=builder /tmp/mycloud-version /etc/mycloud/version
COPY --from=builder /out/mycloud_backend /usr/local/bin/mycloud_backend

RUN mkdir -p /data/files/tmp /data/logs /etc/mycloud

EXPOSE 8080

CMD ["mycloud_backend"]
