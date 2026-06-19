# Upload speed & large-video streaming

This documents the performance work for (1) large-file upload throughput and
(2) streaming/seeking large videos in the browser, plus a runbook for locating
any remaining upload bottleneck.

## What changed

### Streaming large videos & files (HTTP Range)

- The encrypted on-disk format is `MCv2` magic + repeated
  `[len][nonce][ciphertext][tag]` chunks. `EncryptStream` now fills each chunk
  with `io.ReadFull`, so every chunk except the last is exactly 4 MiB. That
  uniform layout lets the server seek to any byte offset arithmetically.
- `storage.DecryptRangeToWriter` decrypts only the chunks a byte range touches.
  `storage.SupportsRange` validates the uniform layout via a single `Stat`;
  legacy / non-uniform blobs cleanly fall back to a full `200` stream.
- A shared `serveDecryptedBlob` responder (in `internal/app/storage_helpers.go`)
  adds `Accept-Ranges`, `Content-Length`, single-range parsing, `206` +
  `Content-Range`, and `416` for unsatisfiable ranges. It is wired into
  `:download`, `:preview`, the presigned `/dl/{token}` endpoint, and version
  streaming.
- Frontend: `<video>`/`<audio>` for the current file now stream from a
  **presigned** range URL (`files.presign`) instead of being downloaded into an
  in-memory blob — so a 10 GB video plays and seeks immediately with flat memory
  use, and the 25 MB inline-preview cap no longer blocks video/audio.
  `VideoPlayer` is direct-first and only falls back to HLS transcoding when the
  browser can't decode the codec natively.

Why presigned URLs: a native `<video src>` can't send an `Authorization`
header, and the `/dl/{token}` endpoint is also outside the per-request
`download` rate limiter — so seeking (many range requests) isn't throttled.

### Upload throughput

- tus `chunkSize` raised from 4 MiB to **16 MiB** (`frontend/lib/api.ts`). tus
  waits for each chunk's `204` before sending the next, so fewer/larger chunks
  cut round-trips and per-chunk server work (~4× fewer PATCHes for a 1.4 GB
  file). Tunable.
- `parallelUploads` is intentionally **off**. It requires the tus
  `concatenation` extension, which (a) would fire the finish hook once per
  partial and (b) makes filestore copy every partial into the final blob — an
  extra full-file I/O pass. Over our single HTTP/2 connection, parallel streams
  add no bandwidth on a low-RTT LAN, so a large serial chunk is the better
  lever. The finish hook now guards `hook.Upload.IsPartial` so the server stays
  correct if any client does enable it.
- A timing log was added at the tus finish step (`handlers_tus.go`) reporting
  how long the post-transfer encrypt pass takes — see B0 below.

## B0 — Measure where the upload time goes

A 1.4 GB upload at ~46 Mbps is far below gigabit. Before further changes,
confirm whether the ceiling is the **network/Docker layer** or the **app**.

### 1. Raw LAN ceiling (physical link)

Install `iperf3` on both machines. On the server box:

```bash
iperf3 -s
```

On the uploading machine:

```bash
iperf3 -c <server-lan-ip>          # download direction
iperf3 -c <server-lan-ip> -R       # reverse (upload direction)
```

- ~900+ Mbps → the LAN is gigabit; a ~46 Mbps upload is **app/stack-bound**.
- ~46 Mbps → the wall is **physical/Docker** (a Wi-Fi leg, a 100 Mbps port, or
  Docker Desktop port-forwarding). No app change will beat the link.

### 2. Finish-encrypt cost (app)

Upload a large file through the web UI, then read the backend log:

```bash
docker compose logs backend | grep "tus finish: encrypted assembled upload"
# → bytes=..., encrypt_ms=..., mib_per_s=...
```

The client is blocked on the final PATCH during this pass, so it counts toward
perceived upload time. If `encrypt_ms` is a large fraction of the total upload
time, the finish pass (re-read + re-encrypt) is a real cost → consider B3.
If `mib_per_s` is high (hundreds+), crypto/disk are not the bottleneck.

### 3. Isolate Docker port-forwarding (optional)

Run iperf3 *inside* the backend container's network vs. from the LAN to compare
the host→container hop against the machine→machine hop:

```bash
docker compose exec backend sh -lc 'apk add --no-cache iperf3 2>/dev/null || true; iperf3 -s' &
# from the Docker host:
iperf3 -c 127.0.0.1 -p 5201
```

If host→container is much slower than machine→machine iperf3, Docker Desktop's
port proxy is the limiter → B4.

## B3 — Eliminate the finish re-encrypt pass (only if B0 says so)

tus stores plaintext during transfer, then the finish hook re-reads it and
writes ciphertext (≈ 1.4 GB read + 1.4 GB write for a 1.4 GB upload). The final
`os.Rename` is already same-volume/metadata-only.

Removing the re-read means encrypting **during** `WriteChunk` via a custom tusd
`DataStore`. ⚠️ This breaks tus offset semantics (stored ciphertext bytes ≠
plaintext `Upload-Length`) and needs a plaintext↔ciphertext offset map for
resume. High risk — do it only if B0 shows the finish pass dominates.

## B4 — Infra tuning (only if B0 says network-bound)

- **Confirm wired gigabit end to end.** A single Wi-Fi leg explains ~46 Mbps on
  its own. Check link speed on both NICs and the switch/router ports.
- **Docker Desktop on Windows** publishes ports through a user-space proxy that
  can cap throughput. Try WSL2 **mirrored networking** mode
  (`.wslconfig`: `[wsl2] networkingMode=mirrored`) and re-measure (B0 step 3).
- **nginx** already streams uploads/downloads unbuffered
  (`proxy_request_buffering off`, `proxy_buffering off`, `client_max_body_size
  10G`). To check whether TLS/HTTP-2 framing is the limiter, compare an upload
  over plain HTTP directly to `backend:8080` (inside the compose network) vs.
  through nginx 443.
- Consider jumbo frames (MTU 9000) if every device on the path supports it.

## Verify

```bash
# Backend unit tests (round-trip, range slices, legacy fallback)
go -C backend-go test ./internal/storage/...

# Range request against a presigned URL (grab one from the SPA or the
# POST /api/v2/files/{id}:presign response):
curl -s -D - -o part.bin -H "Range: bytes=1000000-1100000" "<presigned-url>"
#   → HTTP/1.1 206 Partial Content
#   → Content-Range: bytes 1000000-1100000/<size>
#   → Content-Length: 100001
```

Video E2E: upload a multi-GB MP4, open its preview — it should play and seek
immediately, the Network panel should show `206`s (not one giant `200`), and
tab memory should stay flat.
```
