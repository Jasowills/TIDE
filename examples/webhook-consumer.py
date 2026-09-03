"""Minimal webhook consumer (Grilling §5.5): verify HMAC + dedupe on event_id."""
import hashlib
import hmac
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

SECRET = b"s3cr3t"
seen: set[str] = set()

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        event_id = self.headers.get("X-Tide-Event-Id", "")
        ts = self.headers.get("X-Tide-Timestamp", "")
        sig = self.headers.get("X-Tide-Signature", "")
        if abs(time.time() - int(ts or 0)) > 300:
            self.send_response(400); self.end_headers(); return  # stale: replay?
        want = hmac.new(SECRET, f"{ts}.{event_id}.".encode() + body, hashlib.sha256).hexdigest()
        if not hmac.compare_digest(want, sig):
            self.send_response(401); self.end_headers(); return
        if event_id in seen:
            self.send_response(200); self.end_headers(); return  # at-least-once: dedupe
        seen.add(event_id)
        print(f"event {event_id}: {body[:200]}")
        self.send_response(200); self.end_headers()

if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 8099), Handler).serve_forever()
