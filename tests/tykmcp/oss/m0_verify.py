#!/usr/bin/env python3
"""M0: verify the Tyk OSS Gateway behaviours the OSS MCP backend relies on.

Runs against tests/tykmcp/oss/docker-compose.yml (gw1 :18181, gw2 :18182,
shared Redis) and the demo MCP upstream (tests/tykmcp/mcpsrv) on host port
4021. Prints one line per check and exits non-zero on the first failure
unless --keep-going is given.
"""
import json
import sys
import time
import urllib.error
import urllib.request

SECRET = "oss-secret"
GW = {"gw1": "http://localhost:18181", "gw2": "http://localhost:18182"}
UPSTREAM = "http://host.docker.internal:4021"
KEEP_GOING = "--keep-going" in sys.argv
failures = []
LAST_HEADERS = {}
SESSIONS = {}


def req(base, method, path, body=None, headers=None, secret=True):
    h = {"Content-Type": "application/json"}
    if secret:
        h["X-Tyk-Authorization"] = SECRET
    h.update(headers or {})
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(base + path, data=data, method=method, headers=h)
    try:
        with urllib.request.urlopen(r, timeout=15) as resp:
            raw = resp.read().decode()
            LAST_HEADERS.clear()
            LAST_HEADERS.update({k.lower(): v for k, v in resp.headers.items()})
            return resp.status, raw
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def j(raw):
    try:
        return json.loads(raw)
    except Exception:
        return raw


def check(name, ok, detail=""):
    print(("PASS " if ok else "FAIL ") + name + ("" if ok else "  -> " + str(detail)[:400]))
    if not ok:
        failures.append(name)
        if not KEEP_GOING:
            sys.exit(1)


def mcp_doc(api_id, listen, header_value="Bearer upstream-literal", tools=None):
    mw = {"global": {"trafficLogs": {"enabled": True},
                     "transformRequestHeaders": {"enabled": True, "add": [{"name": "X-Upstream-Token", "value": header_value}]}}}
    if tools:
        mw["mcpTools"] = {t: {"allow": {"enabled": True}} for t in tools}
    return {
        "openapi": "3.0.3",
        "info": {"title": api_id, "version": "2025-11-25"},
        "paths": {"/mcp": {"post": {"operationId": "mcpTransportPost", "responses": {"200": {"description": "ok"}}},
                           "get": {"operationId": "mcpSSEGet", "responses": {"200": {"description": "ok"}}}}},
        "components": {"securitySchemes": {"authToken": {"type": "apiKey", "in": "header", "name": "Authorization"}}},
        "security": [{"authToken": []}],
        "x-tyk-api-gateway": {
            "info": {"id": api_id, "name": api_id, "state": {"active": True}},
            "server": {"listenPath": {"value": listen, "strip": True},
                       "authentication": {"enabled": True, "securitySchemes": {"authToken": {"enabled": True}}}},
            "upstream": {"url": UPSTREAM},
            "middleware": mw,
        },
    }


def mcp_call(gw, listen, key, method, params=None, rid=1):
    body = {"jsonrpc": "2.0", "id": rid, "method": method, "params": params or {}}
    h = {"Authorization": key, "Accept": "application/json, text/event-stream"}
    if method != "initialize" and (listen, key) in SESSIONS:
        h["Mcp-Session-Id"] = SESSIONS[(listen, key)]
    st, raw = req(GW[gw], "POST", listen + "mcp", body, secret=False, headers=h)
    if method == "initialize" and st == 200 and "mcp-session-id" in LAST_HEADERS:
        SESSIONS[(listen, key)] = LAST_HEADERS["mcp-session-id"]
    return st, raw


def rpc_result(raw):
    # Streamable HTTP may answer as SSE; take the last data: line.
    if "data:" in raw:
        lines = [l[5:].strip() for l in raw.splitlines() if l.startswith("data:")]
        raw = lines[-1] if lines else raw
    return j(raw)


def reload(gw):
    return req(GW[gw], "GET", "/tyk/reload?block=true")


def cleanup():
    for gw in GW:
        st, raw = req(GW[gw], "GET", "/tyk/mcps")
        for d in j(raw) or []:
            aid = ((d.get("x-tyk-api-gateway") or {}).get("info") or {}).get("id", "")
            if aid.startswith("m0-"):
                req(GW[gw], "DELETE", "/tyk/mcps/" + aid)
        reload(gw)


cleanup()

# 1. Health and version.
for gw in GW:
    st, raw = req(GW[gw], "GET", "/hello", secret=False)
    check(f"{gw} /hello reports a version", st == 200 and j(raw).get("version"), raw)

# 2. dryRun validates without writing.
doc = mcp_doc("m0-weather", "/m0-weather/")
st, raw = req(GW["gw1"], "POST", "/tyk/mcps?dryRun=true", doc)
check("dryRun create answers 200 with the document", st == 200 and "x-tyk-api-gateway" in raw, (st, raw))
reload("gw1")
st, raw = req(GW["gw1"], "GET", "/tyk/mcps/m0-weather")
check("dryRun did not persist", st == 404, (st, raw))
st, raw = req(GW["gw1"], "POST", "/tyk/mcps?dryRun=true", {"openapi": "3.0.3", "info": {"title": "x", "version": "1"}, "paths": {}})
check("dryRun rejects a document without x-tyk-api-gateway (400)", st == 400, (st, raw))

# 3. A write lands on the answering node only, and needs a reload.
st, raw = req(GW["gw1"], "POST", "/tyk/mcps", doc)
check("create on gw1", st == 200 and j(raw).get("action") == "added", (st, raw))
st, _ = req(GW["gw1"], "GET", "/tyk/mcps/m0-weather")
print(f"INFO gw1 GET before reload -> {st}")
reload("gw1")
st, _ = req(GW["gw1"], "GET", "/tyk/mcps/m0-weather")
check("gw1 serves the definition after reload", st == 200, st)
req(GW["gw1"], "GET", "/tyk/reload/group")
time.sleep(3)
st, _ = req(GW["gw2"], "GET", "/tyk/mcps/m0-weather")
check("gw2 does NOT have it even after a group reload (per-node disk)", st == 404, st)

# 4. Push to gw2 too.
st, raw = req(GW["gw2"], "POST", "/tyk/mcps", doc)
check("create on gw2", st == 200, (st, raw))
reload("gw2")
st, raw = req(GW["gw2"], "GET", "/tyk/mcps/m0-weather")
check("gw2 serves the definition after reload", st == 200, st)
got = j(raw)
check("GET returns the info.id we chose", got.get("x-tyk-api-gateway", {}).get("info", {}).get("id") == "m0-weather", raw[:300])
st, raw = req(GW["gw2"], "POST", "/tyk/mcps", doc)
print(f"INFO creating the same id again -> {st} {raw[:160]}")

# 5. Inline access rights on a key, created via gw1, used via gw2.
session = {
    "alias": "m0-app", "org_id": "", "rate": 1000, "per": 60, "quota_max": -1, "expires": 0,
    "tags": ["ai-studio"], "meta_data": {"studio_app_id": "1"},
    "access_rights": {"m0-weather": {
        "api_id": "m0-weather", "api_name": "m0-weather", "versions": ["Default"],
        "mcp_access_rights": {"tools": {"allowed": ["get-weather"]}},
    }},
}
st, raw = req(GW["gw1"], "POST", "/tyk/keys", session)
kr = j(raw)
check("create key with inline mcp_access_rights", st == 200 and kr.get("key"), (st, raw))
key, key_hash = kr.get("key"), kr.get("key_hash")
check("key create returns key_hash (hash_keys=true)", bool(key_hash), raw)

st, raw = mcp_call("gw2", "/m0-weather/", key, "initialize",
                   {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "m0", "version": "1"}})
check("initialize via gw2 with the key", st == 200, (st, raw))
st, raw = mcp_call("gw2", "/m0-weather/", key, "tools/list", rid=2)
names = [t["name"] for t in (rpc_result(raw).get("result") or {}).get("tools", [])] if st == 200 else []
check("tools/list via gw2 is filtered to the allowed tool", names == ["get-weather"], (st, raw[:400]))
st, raw = mcp_call("gw2", "/m0-weather/", key, "tools/call", {"name": "get-forecast", "arguments": {"city": "x"}}, rid=3)
res = rpc_result(raw)
check("tools/call of a blocked tool is refused", st >= 400 or (isinstance(res, dict) and res.get("error")), (st, raw[:300]))
st, raw = mcp_call("gw1", "/m0-weather/", key, "tools/call", {"name": "get-weather", "arguments": {"city": "Oslo"}}, rid=4)
check("tools/call of the allowed tool works via gw1", st == 200 and "Oslo" in raw, (st, raw[:300]))
st, raw = mcp_call("gw1", "/m0-weather/", "nonsense", "tools/list", rid=5)
check("an unknown key is refused", st in (401, 403), (st, raw[:200]))

# 6. Key management by hash through the other node.
st, raw = req(GW["gw2"], "GET", f"/tyk/keys/{key_hash}?hashed=true")
check("GET key by hash via gw2", st == 200 and "m0-weather" in raw, (st, raw[:200]))
sess = j(raw)
sess["access_rights"]["m0-weather"]["mcp_access_rights"] = {"tools": {"allowed": ["get-weather", "get-forecast"]}}
st, raw = req(GW["gw2"], "PUT", f"/tyk/keys/{key_hash}?hashed=true", sess)
check("PUT key by hash via gw2 (widen tools)", st == 200, (st, raw[:200]))
st, raw = mcp_call("gw1", "/m0-weather/", key, "tools/list", rid=6)
names = sorted(t["name"] for t in (rpc_result(raw).get("result") or {}).get("tools", [])) if st == 200 else []
check("widened rights apply on gw1 at once", names == ["get-forecast", "get-weather"], (st, raw[:300]))
sess["is_inactive"] = True
req(GW["gw2"], "PUT", f"/tyk/keys/{key_hash}?hashed=true", sess)
st, raw = mcp_call("gw1", "/m0-weather/", key, "tools/list", rid=7)
check("is_inactive suspends the key", st in (401, 403), (st, raw[:200]))
st, raw = req(GW["gw1"], "POST", "/tyk/keys/preview", {"access_rights": {}, "org_id": ""})
print(f"INFO /tyk/keys/preview -> {st} {raw[:120]}")

# 7. Upstream header injection: literal and $secret_env.
st, raw = req(GW["gw1"], "POST", "/tyk/mcps", mcp_doc("m0-secret", "/m0-secret/", "$secret_env.DEMO_UPSTREAM"))
check("create a proxy whose upstream header is a $secret_env reference", st == 200, (st, raw))
reload("gw1")
st, raw = req(GW["gw1"], "POST", "/tyk/keys", {**session, "access_rights": {"m0-secret": {"api_id": "m0-secret", "api_name": "m0-secret", "versions": ["Default"]}}})
k2 = j(raw).get("key")
mcp_call("gw1", "/m0-secret/", k2, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "m0", "version": "1"}}, rid=8)
print("INFO check the mcpsrv log for x-upstream-token=\"Bearer from-env-secret\" (secret) and \"Bearer upstream-literal\" (literal)")

# 8. A key for an API one node lacks.
st, raw = mcp_call("gw2", "/m0-secret/", k2, "tools/list", rid=9)
check("node without the definition answers 404 on its listen path", st == 404, (st, raw[:200]))

# 9. Delete by hash, then the key no longer works.
st, raw = req(GW["gw1"], "DELETE", f"/tyk/keys/{key_hash}?hashed=true")
check("DELETE key by hash", st == 200, (st, raw[:200]))
st, raw = req(GW["gw2"], "GET", f"/tyk/keys/{key_hash}?hashed=true")
check("deleted key is gone on gw2", st == 404, (st, raw[:200]))

# 10. List shape.
st, raw = req(GW["gw1"], "GET", "/tyk/mcps")
lst = j(raw)
check("GET /tyk/mcps returns a JSON array of OAS documents", isinstance(lst, list) and all("x-tyk-api-gateway" in d for d in lst), raw[:200])
st, raw = req(GW["gw1"], "GET", "/tyk/apis/oas")
ids = [((d.get("x-tyk-api-gateway") or {}).get("info") or {}).get("id") for d in (j(raw) or [])]
print(f"INFO /tyk/apis/oas lists MCP proxies too: {'m0-weather' in ids}")

# 11. Update and delete.
doc2 = mcp_doc("m0-weather", "/m0-weather/", tools=["get-weather"])
st, raw = req(GW["gw1"], "PUT", "/tyk/mcps/m0-weather", doc2)
check("PUT /tyk/mcps/{id}", st == 200, (st, raw))
st, raw = req(GW["gw1"], "DELETE", "/tyk/mcps/m0-weather")
check("DELETE /tyk/mcps/{id}", st == 200, (st, raw))
st, raw = req(GW["gw1"], "DELETE", "/tyk/mcps/m0-never")
print(f"INFO delete of an unknown id -> {st} {raw[:120]}")

# 12. stripAuthorizationData keeps the consumer's Tyk key away from the upstream.
doc3 = mcp_doc("m0-strip", "/m0-strip/", "Bearer strip-check")
doc3["x-tyk-api-gateway"]["server"]["authentication"]["stripAuthorizationData"] = True
st, raw = req(GW["gw1"], "POST", "/tyk/mcps", doc3)
check("create a proxy with stripAuthorizationData", st == 200, (st, raw))
reload("gw1")
st, raw = req(GW["gw1"], "POST", "/tyk/keys", {**session, "access_rights": {"m0-strip": {"api_id": "m0-strip", "api_name": "m0-strip", "versions": ["Default"]}}})
k3 = j(raw).get("key")
st, raw = mcp_call("gw1", "/m0-strip/", k3, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "m0", "version": "1"}}, rid=10)
check("initialize through the stripping proxy", st == 200, (st, raw[:200]))
print("INFO the mcpsrv log line with x-upstream-token=\"Bearer strip-check\" must show upstream-auth=\"\"")

cleanup()
print("\nfailures:", failures or "none")
sys.exit(1 if failures else 0)
