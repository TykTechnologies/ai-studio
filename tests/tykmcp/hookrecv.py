"""Tiny webhook receiver: appends every POST (topic header + body) to hooks.jsonl."""
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

SP = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(SP, "hooks.jsonl")


class H(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(n).decode("utf-8", "replace")
        with open(OUT, "a") as fh:
            fh.write(json.dumps({"topic": self.headers.get("X-Webhook-Topic"), "path": self.path, "body": body}) + "\n")
        self.send_response(200)
        self.end_headers()

    def log_message(self, *a):
        pass


port = int(sys.argv[1]) if len(sys.argv) > 1 else 4021
HTTPServer(("0.0.0.0", port), H).serve_forever()
