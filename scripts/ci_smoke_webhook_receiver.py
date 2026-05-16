#!/usr/bin/env python3
"""Minimal HTTP POST receiver for CI webhook smoke (background process)."""
import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

HITS_PATH = "/tmp/webhook_hits.json"
hits: list[bytes] = []


class H(BaseHTTPRequestHandler):
    def do_POST(self) -> None:
        n = int(self.headers.get("Content-Length", 0))
        hits.append(self.rfile.read(n))
        with open(HITS_PATH, "w", encoding="utf-8") as f:
            f.write(json.dumps([h.decode() for h in hits]))
        self.send_response(200)
        self.end_headers()

    def log_message(self, *_args) -> None:
        pass


def main() -> None:
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9876
    srv = HTTPServer(("127.0.0.1", port), H)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    import time

    while True:
        time.sleep(3600)


if __name__ == "__main__":
    main()
