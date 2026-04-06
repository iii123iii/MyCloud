#!/usr/bin/env python3
"""
Lightweight HTTP server for the MyCloud updater container.

Endpoints
---------
GET  /health  -> 200  {"status":"ok"}
GET  /status  -> 200  current update state
POST /update  -> 202  triggers an update (JSON body: target_version, current_version)
GET  /log     -> 200  last N lines of the update log
"""

import http.server
import json
import os
import subprocess
import sys
import threading
import time

PORT = int(os.environ.get("UPDATER_PORT", "9999"))
LOG_PATH = os.environ.get("MYCLOUD_UPDATE_LOG_PATH", "/data/logs/update.log")

# ── Shared state ──────────────────────────────────────────────────────────────

_lock = threading.Lock()
_state = {
    "status": "idle",
    "message": "",
    "in_progress": False,
    "target_version": "",
    "log_path": LOG_PATH,
    "started_at": "",
    "finished_at": "",
}


def _set_state(**kwargs):
    with _lock:
        _state.update(kwargs)


def _get_state():
    with _lock:
        return dict(_state)


# ── Update worker ─────────────────────────────────────────────────────────────

def _run_update(target_version, current_version):
    """Execute update.sh and track its outcome."""
    _set_state(
        status="running",
        message=f"Updating to {target_version}...",
        in_progress=True,
        target_version=target_version,
        started_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        finished_at="",
    )

    env = os.environ.copy()
    env["MYCLOUD_UPDATE_TARGET_VERSION"] = target_version
    env["MYCLOUD_UPDATE_CURRENT_VERSION"] = current_version

    try:
        os.makedirs(os.path.dirname(LOG_PATH), exist_ok=True)
        with open(LOG_PATH, "a") as log:
            log.write(f"\n{'=' * 60}\n")
            log.write(f"Update to {target_version} started at "
                       f"{time.strftime('%Y-%m-%d %H:%M:%S UTC')}\n")
            log.write(f"{'=' * 60}\n\n")
            log.flush()

            result = subprocess.run(
                ["/updater/update.sh"],
                env=env,
                stdout=log,
                stderr=subprocess.STDOUT,
                timeout=600,  # 10 minute hard limit
            )

        if result.returncode == 0:
            _set_state(
                status="succeeded",
                message=f"Successfully updated to {target_version}.",
                in_progress=False,
                finished_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            )
        else:
            _set_state(
                status="failed",
                message=(f"Update to {target_version} failed with exit code "
                         f"{result.returncode}. Check {LOG_PATH} for details."),
                in_progress=False,
                finished_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            )
    except subprocess.TimeoutExpired:
        _set_state(
            status="failed",
            message=f"Update to {target_version} timed out after 10 minutes.",
            in_progress=False,
            finished_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        )
    except Exception as exc:
        _set_state(
            status="failed",
            message=f"Update to {target_version} failed: {exc}",
            in_progress=False,
            finished_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        )


# ── HTTP handler ──────────────────────────────────────────────────────────────

class Handler(http.server.BaseHTTPRequestHandler):

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"status": "ok"})
        elif self.path == "/status":
            self._json(200, _get_state())
        elif self.path.startswith("/log"):
            self._serve_log()
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        if self.path == "/update":
            self._handle_update()
        else:
            self._json(404, {"error": "not found"})

    # ── Helpers ───────────────────────────────────────────────────────────────

    def _handle_update(self):
        state = _get_state()
        if state["in_progress"]:
            self._json(409, {"error": "An update is already in progress."})
            return

        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length)) if length > 0 else {}
        target = body.get("target_version", "")
        current = body.get("current_version", "")

        if not target:
            self._json(400, {"error": "target_version is required."})
            return

        thread = threading.Thread(
            target=_run_update, args=(target, current), daemon=True,
        )
        thread.start()

        self._json(202, {
            "message": f"Update to {target} started. Logs: {LOG_PATH}",
            "log_path": LOG_PATH,
        })

    def _serve_log(self):
        lines = 200
        try:
            with open(LOG_PATH, "r") as f:
                all_lines = f.readlines()
                tail = all_lines[-lines:]
            self._json(200, {"lines": [l.rstrip("\n") for l in tail]})
        except FileNotFoundError:
            self._json(200, {"lines": []})

    def _json(self, code, data):
        body = json.dumps(data).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        # Log to stdout instead of stderr for Docker log collection.
        print(f"[updater] {fmt % args}", flush=True)


# ── Main ──────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    server = http.server.HTTPServer(("0.0.0.0", PORT), Handler)
    print(f"[updater] listening on :{PORT}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    server.server_close()
