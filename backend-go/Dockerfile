FROM golang:1.26.1-bookworm AS builder

ARG MYCLOUD_VERSION

WORKDIR /src
COPY backend-go/go.mod backend-go/go.sum ./backend-go/
WORKDIR /src/backend-go
RUN go mod download

WORKDIR /src
COPY . .

RUN set -eu; \
    # The repo is COPYed in as root; git refuses to read a repo it considers
    # "dubiously owned" and exits 128, which would silently drop the version
    # fallback below to "dev". Trust the build tree so `git describe` works.
    git config --global --add safe.directory '*' 2>/dev/null || true; \
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
# -buildvcs=false: the app version comes from /etc/mycloud/version (set above),
# not Go's VCS stamping, so don't make the build depend on git being able to
# read the copied .git (it can't reliably in-container → "error obtaining VCS
# status: exit status 128").
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/mycloud_backend ./cmd/api

FROM debian:bookworm-slim

# ffmpeg is required for HLS adaptive video streaming. Adds ~120 MB
# to the runtime image — acceptable for the in-process transcoder design.
# Operators who don't need HLS can swap to a slimmer base; the binary
# tolerates ffmpeg's absence at runtime (HLS endpoints return 503).
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates tzdata ffmpeg \
    && rm -rf /var/lib/apt/lists/*

ENV MYCLOUD_VERSION_FILE=/etc/mycloud/version

COPY --from=builder /tmp/mycloud-version /etc/mycloud/version
COPY --from=builder /out/mycloud_backend /usr/local/bin/mycloud_backend

RUN mkdir -p /data/files/tmp /data/logs /etc/mycloud

EXPOSE 8080

CMD ["mycloud_backend"]
