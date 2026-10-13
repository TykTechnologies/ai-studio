#!/usr/bin/env python3
"""Recording mock LLM upstream for the tool-result guardrail E2E (4uc.10).

Speaks just enough of each vendor wire format to answer the gateway, and
records every request body it receives, so the suite asserts what actually
reached the vendor rather than what the gateway answered.

    python3 mockupstream.py PORT RECORD_FILE

Routes are matched by path suffix, so any base URL works:

    .../chat/completions                OpenAI Chat Completions (stream or not)
    .../responses                       OpenAI Responses
    .../messages                        Anthropic Messages (stream or not)
    .../models/<m>:generateContent      Gemini
    .../models/<m>:streamGenerateContent  Gemini streaming (alt=sse or JSON array)
    /tool/<name>                        a REST tool for Studio chat: returns the
                                        body set with POST /_tool (status too)
    GET  /_recorded?since=N             recorded requests from index N
    POST /_reset                        forget the recorded requests
    POST /_tool {"status":200,"body":"..."}  what /tool/* answers

Like the vendors, it rejects a conversation whose tool calls and tool results
do not pair up (400), so a guardrail rewrite that breaks the shape fails the
suite instead of passing silently.

Chat behaviour for Studio chat: when an OpenAI request offers tools and its
last message is a user message containing "use the tool", the answer is a
tool call to the first tool; when the last message is a tool result, the
answer is "done" plus the first 60 characters of that result.

Standard library only.
"""

import json
import re
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

LOCK = threading.Lock()
RECORDED = []
TOOL = {"status": 200, "body": '{"ok": true}'}
RECORD_FILE = None


def record(entry):
    with LOCK:
        entry["index"] = len(RECORDED)
        RECORDED.append(entry)
        if RECORD_FILE:
            with open(RECORD_FILE, "a") as f:
                f.write(json.dumps(entry) + "\n")


# ---- tool pairing checks (what the vendors reject) ----------------------------------

def openai_pairing(msgs):
    open_ids = set()
    for m in msgs:
        if m.get("role") == "assistant":
            for c in m.get("tool_calls") or []:
                open_ids.add(c.get("id"))
        elif m.get("role") == "tool":
            tid = m.get("tool_call_id")
            if tid not in open_ids:
                return f"tool message answers unknown tool_call_id {tid!r}"
            open_ids.discard(tid)
    return None


def anthropic_pairing(msgs):
    pending = set()
    for m in msgs:
        content = m.get("content")
        blocks = content if isinstance(content, list) else []
        if m.get("role") == "assistant":
            # Server tool results sit in the same assistant turn as their use.
            uses = {b.get("id") for b in blocks if isinstance(b, dict) and b.get("type") in ("server_tool_use", "mcp_tool_use")}
            for b in blocks:
                if isinstance(b, dict) and b.get("type", "").endswith("_tool_result") and b.get("tool_use_id") not in uses:
                    return f"{b.get('type')} answers unknown id {b.get('tool_use_id')!r}"
            pending = {b.get("id") for b in blocks if isinstance(b, dict) and b.get("type") == "tool_use"}
        else:
            answered = {b.get("tool_use_id") for b in blocks if isinstance(b, dict) and b.get("type") == "tool_result"}
            if answered - pending:
                return f"tool_result for unknown tool_use_id {sorted(answered - pending)}"
            if pending and pending - answered:
                return f"tool_use ids without a tool_result: {sorted(pending - answered)}"
            pending = set()
    return None


def gemini_pairing(contents):
    calls = []
    for c in contents:
        parts = c.get("parts") or []
        if c.get("role") == "model":
            calls = [p["functionCall"].get("name") for p in parts if isinstance(p, dict) and "functionCall" in p]
            continue
        responses = [p["functionResponse"].get("name") for p in parts if isinstance(p, dict) and "functionResponse" in p]
        if responses:
            if sorted(responses) != sorted(calls):
                return f"functionResponse names {responses} do not answer functionCall names {calls}"
            calls = []
    return None


def responses_pairing(items):
    if not isinstance(items, list):
        return None
    calls = set()
    for it in items:
        if not isinstance(it, dict):
            continue
        t = it.get("type", "")
        if t in ("function_call", "custom_tool_call"):
            calls.add(it.get("call_id"))
        elif t in ("function_call_output", "custom_tool_call_output") and it.get("call_id") not in calls:
            return f"{t} answers unknown call_id {it.get('call_id')!r}"
    return None


# ---- the server ----------------------------------------------------------------------

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def read_body(self):
        # The gateway's loopback hop can re-send without Content-Length.
        if self.headers.get("Transfer-Encoding", "").lower() == "chunked":
            data = b""
            while True:
                size = int(self.rfile.readline().strip() or b"0", 16)
                if size == 0:
                    self.rfile.readline()
                    break
                data += self.rfile.read(size)
                self.rfile.readline()
            return data
        n = int(self.headers.get("Content-Length") or 0)
        return self.rfile.read(n) if n else b""

    def send_json(self, status, obj):
        b = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def send_sse(self, events, done=True):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()
        for ev in events:
            if isinstance(ev, tuple):
                self.wfile.write(f"event: {ev[0]}\ndata: {json.dumps(ev[1])}\n\n".encode())
            else:
                self.wfile.write(f"data: {json.dumps(ev)}\n\n".encode())
            self.wfile.flush()
        if done:
            self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()
        self.close_connection = True

    def do_GET(self):
        u = urlparse(self.path)
        if u.path == "/_recorded":
            since = int(parse_qs(u.query).get("since", ["0"])[0])
            with LOCK:
                self.send_json(200, RECORDED[since:])
            return
        if u.path in ("/health", "/healthz"):
            self.send_json(200, {"ok": True})
            return
        if u.path.startswith("/tool/"):
            self.tool(u, b"")
            return
        self.send_json(404, {"error": "not found"})

    def tool(self, u, raw):
        record({"ts": time.time(), "path": u.path, "method": self.command, "kind": "tool", "body": raw.decode("utf-8", "replace")})
        b = TOOL["body"].encode()
        self.send_response(TOOL["status"])
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def do_POST(self):
        u = urlparse(self.path)
        raw = self.read_body()
        if u.path == "/_reset":
            with LOCK:
                RECORDED.clear()
            self.send_json(200, {"ok": True})
            return
        if u.path == "/_tool":
            cfg = json.loads(raw or b"{}")
            TOOL["status"] = int(cfg.get("status", 200))
            TOOL["body"] = cfg.get("body", "")
            self.send_json(200, {"ok": True})
            return
        if u.path.startswith("/tool/"):
            self.tool(u, raw)
            return
        try:
            body = json.loads(raw) if raw else {}
        except ValueError:
            body = None
        entry = {"ts": time.time(), "path": u.path, "query": u.query, "method": "POST", "raw": raw.decode("utf-8", "replace"), "body": body}
        p = u.path
        if p == "/classify":
            # An HTTP guardrail classifier (the http provider's contract) that
            # flags a call holding a canary without saying which segment:
            # an unpinned finding (D4).
            entry["kind"] = "classifier"
            record(entry)
            segs = (body or {}).get("segments") or []
            hit = any("[canary:" in (s.get("text") or "") for s in segs)
            findings = [{"detector": "prompt_attack", "category": "injection", "score": 0.99, "segment_index": -1}] if hit else []
            return self.send_json(200, {"flagged": hit, "findings": findings})
        if p.endswith("/files"):
            # OpenAI Files: a multipart upload (purpose=batch, a JSONL file).
            entry["kind"] = "openai_file"
            record(entry)
            return self.send_json(200, {"id": "file-mock", "object": "file", "bytes": len(raw), "created_at": int(time.time()),
                                        "filename": "batch.jsonl", "purpose": "batch", "status": "processed"})
        if p.endswith("/messages/batches"):
            entry["kind"] = "anthropic_batch"
            record(entry)
            n = len((body or {}).get("requests") or [])
            return self.send_json(200, {"id": "msgbatch_mock", "type": "message_batch", "processing_status": "in_progress",
                                        "request_counts": {"processing": n, "succeeded": 0, "errored": 0, "canceled": 0, "expired": 0},
                                        "created_at": "2026-10-13T00:00:00Z", "expires_at": "2026-10-14T00:00:00Z", "ended_at": None,
                                        "cancel_initiated_at": None, "archived_at": None, "results_url": None})
        if p.endswith("/chat/completions"):
            entry["kind"] = "openai"
            record(entry)
            return self.openai_chat(body or {})
        if p.endswith("/responses"):
            entry["kind"] = "openai_responses"
            record(entry)
            return self.openai_responses(body or {})
        if p.endswith("/messages"):
            entry["kind"] = "anthropic"
            record(entry)
            return self.anthropic(body or {})
        m = re.search(r"/models/([^/:]+):(generateContent|streamGenerateContent)$", p)
        if m:
            entry["kind"] = "google"
            record(entry)
            return self.gemini(body or {}, m.group(2) == "streamGenerateContent", "alt=sse" in u.query)
        entry["kind"] = "unknown"
        record(entry)
        self.send_json(404, {"error": f"no mock route for {p}"})

    # ---- OpenAI Chat Completions ----
    def openai_chat(self, body):
        msgs = body.get("messages") or []
        err = openai_pairing(msgs)
        if err:
            return self.send_json(400, {"error": {"message": err, "type": "invalid_request_error"}})
        model = body.get("model", "gpt-mock")
        last = msgs[-1] if msgs else {}
        tools = body.get("tools") or []
        tool_call = None
        text = "ok"
        if last.get("role") == "tool":
            c = last.get("content")
            if isinstance(c, list):
                c = " ".join(p.get("text", "") for p in c if isinstance(p, dict))
            text = "done: " + (c or "")[:60]
        elif tools and last.get("role") == "user" and "use the tool" in json.dumps(last.get("content")):
            fn = tools[0].get("function", {}).get("name", "tool")
            tool_call = {"id": f"call_mock_{int(time.time() * 1000) % 100000}", "type": "function", "function": {"name": fn, "arguments": "{}"}}
        usage = {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
        if body.get("stream"):
            delta = {"role": "assistant"}
            if tool_call:
                delta["tool_calls"] = [dict(tool_call, index=0)]
            else:
                delta["content"] = text
            events = [
                {"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": int(time.time()), "model": model, "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
                {"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": int(time.time()), "model": model, "choices": [{"index": 0, "delta": {}, "finish_reason": "tool_calls" if tool_call else "stop"}], "usage": usage},
            ]
            return self.send_sse(events)
        msg = {"role": "assistant", "content": None if tool_call else text}
        if tool_call:
            msg["tool_calls"] = [tool_call]
        self.send_json(200, {"id": "chatcmpl-mock", "object": "chat.completion", "created": int(time.time()), "model": model,
                             "choices": [{"index": 0, "message": msg, "finish_reason": "tool_calls" if tool_call else "stop"}], "usage": usage})

    # ---- OpenAI Responses ----
    def openai_responses(self, body):
        err = responses_pairing(body.get("input"))
        if err:
            return self.send_json(400, {"error": {"message": err, "type": "invalid_request_error"}})
        self.send_json(200, {"id": "resp_mock", "object": "response", "created_at": int(time.time()), "status": "completed", "model": body.get("model", "gpt-mock"),
                             "output": [{"type": "message", "id": "msg_mock", "status": "completed", "role": "assistant", "content": [{"type": "output_text", "text": "ok", "annotations": []}]}],
                             "usage": {"input_tokens": 10, "output_tokens": 2, "total_tokens": 12}})

    # ---- Anthropic Messages ----
    def anthropic(self, body):
        err = anthropic_pairing(body.get("messages") or [])
        if err:
            return self.send_json(400, {"type": "error", "error": {"type": "invalid_request_error", "message": err}})
        model = body.get("model", "claude-mock")
        if body.get("stream"):
            events = [
                ("message_start", {"type": "message_start", "message": {"id": "msg_mock", "type": "message", "role": "assistant", "model": model, "content": [], "stop_reason": None, "usage": {"input_tokens": 10, "output_tokens": 1}}}),
                ("content_block_start", {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}),
                ("content_block_delta", {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "ok"}}),
                ("content_block_stop", {"type": "content_block_stop", "index": 0}),
                ("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {"output_tokens": 2}}),
                ("message_stop", {"type": "message_stop"}),
            ]
            return self.send_sse(events, done=False)
        self.send_json(200, {"id": "msg_mock", "type": "message", "role": "assistant", "model": model, "content": [{"type": "text", "text": "ok"}],
                             "stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 2}})

    # ---- Gemini ----
    def gemini(self, body, stream, sse):
        err = gemini_pairing(body.get("contents") or [])
        if err:
            return self.send_json(400, {"error": {"code": 400, "message": err, "status": "INVALID_ARGUMENT"}})
        resp = {"candidates": [{"content": {"role": "model", "parts": [{"text": "ok"}]}, "finishReason": "STOP", "index": 0}],
                "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 2, "totalTokenCount": 12}, "modelVersion": "gemini-mock"}
        if stream and sse:
            return self.send_sse([resp], done=False)
        if stream:
            return self.send_json(200, [resp])
        self.send_json(200, resp)


def main():
    global RECORD_FILE
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 14021
    RECORD_FILE = sys.argv[2] if len(sys.argv) > 2 else None
    srv = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    print(f"mock upstream on :{port}", flush=True)
    srv.serve_forever()


if __name__ == "__main__":
    main()
