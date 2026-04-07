FROM golang:1.26.1-bookworm AS builder

ARG MYCLOUD_VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/mycloud_backend ./cmd/api

FROM debian:bookworm-slim

ARG MYCLOUD_VERSION=dev

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

ENV MYCLOUD_VERSION=${MYCLOUD_VERSION}

COPY --from=builder /out/mycloud_backend /usr/local/bin/mycloud_backend

RUN mkdir -p /data/files/tmp /data/logs

EXPOSE 8080

CMD ["mycloud_backend"]
