FROM golang:1.26.1-alpine AS builder

ARG MYCLOUD_VERSION=v0.0.0-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build \
    -ldflags="-X main.Version=${MYCLOUD_VERSION}" \
    -o mycloud_backend \
    ./cmd/server

# ─── Runtime stage ───────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    git \
    docker-cli \
    mariadb-client

COPY --from=builder /app/mycloud_backend /usr/local/bin/mycloud_backend

# Pre-create the upload temp directory inside the storage volume mountpoint
RUN mkdir -p /data/files/tmp

EXPOSE 8080

CMD ["mycloud_backend"]
