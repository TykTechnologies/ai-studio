#!/usr/bin/env python3
"""Live end-to-end check of Tyk OSS Gateway connections (features/TykOSSGatewayMCP.md).

Runs an enterprise AI Studio against the two-replica OSS cluster in
docker-compose.yml (gw1 :18181, gw2 :18182, gw3 :18183 joins mid-run) and the
demo MCP upstream (tests/tykmcp/mcpsrv on host port 4021). It never prints a
key or a secret.

Environment:
  STUDIO        AI Studio base URL (default http://localhost:8094)
  STUDIO_KEY    an administrator API key; when unset the script registers the
                first user of a fresh Studio (ALLOW_REGISTRATIONS=true,
                DEVMODE=true) and rolls a key
  GW_SECRET     the gateways' secret (default oss-secret)
  UPSTREAM_LOG  the demo MCP server's log, to check upstream headers (optional)
"""
import http.cookiejar
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
STUDIO = os.environ.get("STUDIO", "http://localhost:8094")
SKEY = os.environ.get("STUDIO_KEY", "")
GW_SECRET = os.environ.get("GW_SECRET", "oss-secret")
GW = {"gw1": "http://localhost:18181", "gw2": "http://localhost:18182", "gw3": "http://localhost:18183"}
UPSTREAM = "http://host.docker.internal:4021"
UPSTREAM_LOG = os.environ.get("UPSTREAM_LOG", "")
COMPOSE = ["docker", "compose", "-f", os.path.join(HERE, "docker-compose.yml")]

failures = []
created = {"connections": [], "apps": [], "catalogues": []}
jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
csrf = ""


def http(method, url, body=None, headers=None, raw=False, timeout=30):
    h = {"Content-Type": "application/json", "Accept": "application/json"}
    h.update(headers or {})
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        with opener.open(req, timeout=timeout) as resp:
            text = resp.read().decode()
            hdrs = {k.lower(): v for k, v in resp.headers.items()}
            code = resp.status
    except urllib.error.HTTPError as e:
        text = e.read().decode()
        hdrs = {k.lower(): v for k, v in e.headers.items()}
        code = e.code
    except Exception as e:  # connection refused etc.
        return 0, str(e), {}
    if raw:
        return code, text, hdrs
    try:
        return code, json.loads(text) if text else None, hdrs
    except ValueError:
        return code, text[:500], hdrs


def studio(method, path, body=None):
    if SKEY:
        h = {"Authorization": "Bearer " + SKEY}
    else:
        h = {"X-CSRF-Token": csrf, "Origin": "http://localhost:3000", "Referer": "http://localhost:3000/admin"}
    code, data, _ = http(method, STUDIO + path, body, h)
    return code, data


def gw(node, method, path, body=None):
    code, data, _ = http(method, GW[node] + path, body, {"X-Tyk-Authorization": GW_SECRET})
    return code, data


def check(name, ok, detail=""):
    print(("PASS " if ok else "FAIL ") + name + (("  -- " + str(detail)[:300]) if detail and not ok else ""))
    if not ok:
        failures.append(name)
    return ok


def bootstrap():
    global SKEY, csrf
    if SKEY:
        return
    code, _, h = http("GET", STUDIO + "/csrf-token", raw=True)
    csrf = h.get("x-csrf-token", "")
    reg = {"data": {"type": "users", "attributes": {"email": "oss-e2e@tyk.io", "name": "OSS e2e", "password": "Oss-e2e-Pass#2026", "with_portal": True, "with_chat": True}}}
    studio("POST", "/auth/register", reg)
    code, me = studio("POST", "/auth/login", {"data": {"type": "login", "attributes": {"email": "oss-e2e@tyk.io", "password": "Oss-e2e-Pass#2026"}}})
    if code != 200:
        sys.exit(f"login failed: {code} {me}")
    code, rolled = studio("POST", "/common/me/api-key/roll")
    SKEY = (rolled or {}).get("data", {}).get("api_key", "")
    if not SKEY:
        sys.exit(f"could not roll an API key: {code}")


def mcp(node, path, key, method, params=None, session=None):
    h = {"Accept": "application/json, text/event-stream"}
    if key:
        h["Authorization"] = key
    if session:
        h["Mcp-Session-Id"] = session
    body = {"jsonrpc": "2.0", "id": 1, "method": method, "params": params or {}}
    code, text, hdrs = http("POST", GW[node] + path + "mcp", body, h, raw=True)
    if "data:" in text:
        lines = [l[5:].strip() for l in text.splitlines() if l.startswith("data:")]
        text = lines[-1] if lines else text
    return code, text, hdrs.get("mcp-session-id")


def tools(node, path, key):
    code, _, sid = mcp(node, path, key, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "e2e", "version": "1"}})
    if code != 200:
        return code, []
    code, text, _ = mcp(node, path, key, "tools/list", {}, sid)
    try:
        return code, sorted(t["name"] for t in json.loads(text)["result"]["tools"])
    except Exception:
        return code, []


def operator_proxy(api_id, listen):
    return {
        "openapi": "3.0.3", "info": {"title": api_id, "version": "2025-11-25"},
        "paths": {"/mcp": {"post": {"operationId": "mcpTransportPost", "responses": {"200": {"description": "ok"}}},
                           "get": {"operationId": "mcpSSEGet", "responses": {"200": {"description": "ok"}}}}},
        "components": {"securitySchemes": {"authToken": {"type": "apiKey", "in": "header", "name": "Authorization"}}},
        "security": [{"authToken": []}],
        "x-tyk-api-gateway": {
            "info": {"id": api_id, "name": api_id, "state": {"active": True}},
            "server": {"listenPath": {"value": listen, "strip": True},
                       "authentication": {"enabled": True, "stripAuthorizationData": True, "securitySchemes": {"authToken": {"enabled": True}}}},
            "upstream": {"url": UPSTREAM},
        },
    }


def node_ids(node):
    code, docs = gw(node, "GET", "/tyk/mcps")
    if code != 200 or not isinstance(docs, list):
        return None
    return sorted(((d.get("x-tyk-api-gateway") or {}).get("info") or {}).get("id", "") for d in docs)


def clean_gateways(nodes=("gw1", "gw2", "gw3")):
    for n in nodes:
        ids = node_ids(n)
        if ids is None:
            continue
        for i in ids:
            if i.startswith("studio-") or i.startswith("ops-"):
                gw(n, "DELETE", "/tyk/mcps/" + i)
        gw(n, "GET", "/tyk/reload?block=true")


def sync(cid):
    code, run = studio("POST", f"/api/v1/tyk-connections/{cid}/sync?wait=true")
    return code, run or {}


def servers(cid):
    code, lst = studio("GET", f"/api/v1/mcp-servers?connection_id={cid}&page_size=100")
    return {s["tyk_api_id"]: s for s in (lst or {}).get("servers", [])}


def main():
    bootstrap()
    subprocess.run(COMPOSE + ["--profile", "scale", "stop", "gw3"], capture_output=True)
    clean_gateways(("gw1", "gw2"))

    # ---------- 1. operator proxies: one on every node, one on gw1 only ----------
    for n in ("gw1", "gw2"):
        gw(n, "POST", "/tyk/mcps", operator_proxy("ops-weather", "/ops-weather/"))
    gw("gw1", "POST", "/tyk/mcps", operator_proxy("ops-partial", "/ops-partial/"))
    domain = operator_proxy("ops-domain", "/ops-domain/")
    domain["x-tyk-api-gateway"]["server"]["customDomain"] = {"enabled": True, "name": "mcp.acme.test"}
    for n in ("gw1", "gw2"):
        gw(n, "POST", "/tyk/mcps", domain)
    for n in ("gw1", "gw2"):
        gw(n, "GET", "/tyk/reload?block=true")
    check("1.seeded operator proxies", "ops-weather" in (node_ids("gw2") or []) and "ops-partial" not in (node_ids("gw2") or []))

    # ---------- 2. connection, probe, activation ----------
    code, probe = studio("POST", "/api/v1/tyk-connections/probe", {
        "kind": "gateway", "dashboard_url": GW["gw1"], "dashboard_access_token": GW_SECRET, "declared_mode": "full",
        "allow_internal_host": True, "gateway_discovery": "static", "gateway_node_urls": [GW["gw2"]]})
    check("2.probe unsaved settings", code == 200 and probe.get("effective_mode") == "full", (code, probe))
    code, bad = studio("POST", "/api/v1/tyk-connections/probe", {
        "kind": "gateway", "dashboard_url": GW["gw1"], "dashboard_access_token": "wrong", "declared_mode": "full", "allow_internal_host": True})
    check("2.probe with a wrong secret is not usable", code == 200 and not bad.get("effective_mode"), (code, bad))
    code, conn = studio("POST", "/api/v1/tyk-connections", {
        "name": "OSS e2e", "kind": "gateway", "dashboard_url": GW["gw1"], "dashboard_access_token": GW_SECRET,
        "declared_mode": "full", "allow_internal_host": True, "gateway_base_url": GW["gw1"],
        "gateway_discovery": "static", "gateway_node_urls": [GW["gw2"]]})
    check("2.create gateway connection", code == 201 and conn.get("kind") == "gateway", (code, conn))
    cid = conn["id"]
    created["connections"].append(cid)
    check("2.secret never returned", GW_SECRET not in json.dumps(conn))
    code, conn = studio("POST", f"/api/v1/tyk-connections/{cid}/activate")
    caps = (conn or {}).get("capabilities", {})
    check("2.activate in full mode", code == 200 and conn.get("effective_mode") == "full", (code, conn))
    check("2.shared Redis proven", caps.get("cluster_shared_redis", {}).get("state") == "ok", caps.get("cluster_shared_redis"))
    check("2.both nodes answered", "2 of 2" in caps.get("gateway_nodes", {}).get("detail", ""), caps.get("gateway_nodes"))
    check("2.REST to MCP not offered", caps.get("rest_to_mcp_supported", {}).get("state") == "no")
    prefix = conn.get("gateway_api_id_prefix", "")

    # ---------- 3. discovery ----------
    code, run = sync(cid)
    check("3.sync ok", code == 200 and run.get("status") == "ok", (code, run))
    srvs = servers(cid)
    w, p = srvs.get("ops-weather", {}), srvs.get("ops-partial", {})
    check("3.operator proxy imported as gateway origin", w.get("origin") == "gateway" and w.get("gateway_coverage") == "2/2", w)
    check("3.operator proxy is brokerable without a pin", w.get("brokerable") is True, w)
    check("3.partial proxy flagged and not brokerable", p.get("gateway_partial") is True and p.get("gateway_coverage") == "1/2" and not p.get("brokerable"), p)
    check("3.partial proxy not copied", "ops-partial" not in (node_ids("gw2") or []))
    code, nodes = studio("GET", f"/api/v1/tyk-connections/{cid}/nodes")
    check("3.two nodes in sync", code == 200 and len(nodes) == 2 and all(n["state"] == "in_sync" and n["version"] for n in nodes), nodes)

    # Portal URLs: the connection's public base URL, or the proxy's own custom domain.
    check("3.endpoint from the public base URL", w.get("endpoint_url") == GW["gw1"] + "/ops-weather/mcp", w.get("endpoint_url"))
    d = srvs.get("ops-domain", {})
    check("3.endpoint from the proxy's custom domain", d.get("endpoint_url") == "http://mcp.acme.test/ops-domain/mcp", d.get("endpoint_url"))
    lock = studio("GET", f"/api/v1/tyk-connections/{cid}")[1]["lock_version"]
    studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"gateway_base_url": "https://mcp.example.com", "lock_version": lock})
    sync(cid)
    srvs = servers(cid)
    check("3.base URL change reaches unchanged servers", srvs["ops-weather"].get("endpoint_url") == "https://mcp.example.com/ops-weather/mcp", srvs["ops-weather"].get("endpoint_url"))
    check("3.custom domain follows the new scheme", srvs["ops-domain"].get("endpoint_url") == "https://mcp.acme.test/ops-domain/mcp", srvs["ops-domain"].get("endpoint_url"))
    lock = studio("GET", f"/api/v1/tyk-connections/{cid}")[1]["lock_version"]
    studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"gateway_base_url": GW["gw1"], "lock_version": lock})
    sync(cid)

    # ---------- 4. registration writes every node ----------
    reg = {"connection_id": cid, "kind": "remote", "name": "OSS Weather", "listen_path": "/oss-weather/", "upstream_url": UPSTREAM,
           "upstream_auth_header_name": "X-Upstream-Token", "upstream_auth_token": "Bearer e2e-upstream-token", "consumer_auth": "auth_token",
           "privacy_score": 20}
    code, prev = studio("POST", "/api/v1/mcp-servers/register?dry_run=true", reg)
    preview = (prev or {}).get("preview", prev) or {}
    check("4.preview validated by a gateway node", code == 200 and preview.get("dashboard_validated") is True, (code, prev))
    check("4.preview masks the upstream token", "e2e-upstream-token" not in json.dumps(prev))
    check("4.preview wrote nothing", not any(i.startswith(prefix) for i in (node_ids("gw1") or []) + (node_ids("gw2") or [])))
    code, res = studio("POST", "/api/v1/mcp-servers/register", reg)
    srv = (res or {}).get("server", res) or {}
    check("4.register", code in (200, 201) and srv.get("tyk_api_id", "").startswith(prefix), (code, res))
    api_id = srv.get("tyk_api_id", "")
    check("4.gw1 serves it at once", api_id in (node_ids("gw1") or []))
    check("4.gw2 serves it at once", api_id in (node_ids("gw2") or []))
    code, doc = gw("gw2", "GET", "/tyk/mcps/" + api_id)
    authn = (((doc or {}).get("x-tyk-api-gateway") or {}).get("server") or {}).get("authentication") or {}
    check("4.client Tyk key is stripped before the upstream", authn.get("stripAuthorizationData") is True, authn)
    code, run = sync(cid)
    check("4.sync after registration ok", run.get("status") == "ok", run)
    code, nodes = studio("GET", f"/api/v1/tyk-connections/{cid}/nodes")
    check("4.nodes in sync with 1 studio proxy", all(n["state"] == "in_sync" and n["studio_count"] == 1 for n in nodes), nodes)

    # ---------- 5. publish, catalogue, App, key ----------
    for s in (srv, w):
        studio("PATCH", f"/api/v1/mcp-servers/{s['id']}", {"privacy_score": 20, "lock_version": studio("GET", f"/api/v1/mcp-servers/{s['id']}")[1]["lock_version"]})
        code, pub = studio("POST", f"/api/v1/mcp-servers/{s['id']}/activate")
        check(f"5.publish {s['tyk_api_id'][:12]}", code == 200 and pub.get("is_active"), (code, pub))
    code, cat = studio("POST", "/api/v1/tool-catalogues", {"data": {"type": "ToolCatalogue", "attributes": {"name": "OSS MCP e2e", "short_description": "e2e"}}})
    cat_id = int(cat["data"]["id"])
    created["catalogues"].append(cat_id)
    code, groups = studio("GET", "/api/v1/groups")
    glist = groups if isinstance(groups, list) else (groups or {}).get("data") or []
    for g in glist:
        studio("POST", f"/api/v1/groups/{g['id']}/tool-catalogues", {"data": {"type": "ToolCatalogue", "id": str(cat_id)}})
    for s in (srv, w, p):
        studio("PUT", f"/api/v1/mcp-servers/{s['id']}/catalogues", {"tool_catalogue_ids": [cat_id]})
    code, bad_app = studio("POST", "/common/apps", {"name": "Partial app", "description": "e2e", "data_source_ids": [], "llm_ids": [], "tool_ids": [], "mcp_server_ids": [p["id"]]})
    check("5.binding a partial proxy is refused", code >= 400, (code, bad_app))
    if code < 300 and isinstance(bad_app, dict) and bad_app.get("id"):
        created["apps"].append(bad_app["id"])
    code, app = studio("POST", "/common/apps", {"name": "OSS agent", "description": "e2e", "data_source_ids": [], "llm_ids": [], "tool_ids": [], "mcp_server_ids": [srv["id"], w["id"]]})
    check("5.create App with both servers", code == 201, (code, app))
    app_id = app["id"]
    created["apps"].append(app_id)
    code, _ = studio("POST", f"/api/v1/apps/{app_id}/activate-credential")
    code, minted = studio("POST", f"/common/apps/{app_id}/mcp/credentials", {"connection_id": cid})
    check("5.mint key", code == 201 and minted.get("key"), (code, "no key" if code == 201 else minted))
    key = minted.get("key", "")
    cred = minted.get("credential", {})
    check("5.key carries both proxies inline", sorted(cred.get("applied_policy_ids", [])) == sorted([api_id, "ops-weather"]), cred.get("applied_policy_ids"))
    for n in ("gw1", "gw2"):
        code, names = tools(n, "/oss-weather/", key)
        check(f"5.{n}: key reaches the Studio proxy", code == 200 and names == ["get-forecast", "get-weather"], (code, names))
        code, names = tools(n, "/ops-weather/", key)
        check(f"5.{n}: key reaches the operator proxy", code == 200 and "get-weather" in names, (code, names))
    code, _, _ = mcp("gw1", "/oss-weather/", "", "initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "e2e", "version": "1"}})
    check("5.no key is refused", code in (401, 403), code)
    code, _, _ = mcp("gw1", "/ops-partial/", key, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "e2e", "version": "1"}})
    check("5.key does not reach the partial proxy", code in (401, 403), code)
    if UPSTREAM_LOG and os.path.exists(UPSTREAM_LOG):
        lines = [l for l in open(UPSTREAM_LOG).read().splitlines() if 'x-upstream-token="Bearer e2e-upstream-token"' in l]
        check("5.upstream got its token and not the client's key", lines and all('upstream-auth=""' in l for l in lines), lines[-1:] if lines else "no upstream calls")

    # ---------- 6. key access: tools and limits written onto the live key ----------
    code, ka = studio("PUT", f"/api/v1/mcp-servers/{srv['id']}/key-access", {"allowed_tools": ["get-weather"], "rate": 1000, "per": 60})
    check("6.set key access", code == 200 and ka.get("key_access", {}).get("allowed_tools") == ["get-weather"], (code, ka))
    code, names = tools("gw2", "/oss-weather/", key)
    check("6.tools/list filtered on another node at once", code == 200 and names == ["get-weather"], (code, names))
    code, _, sid = mcp("gw1", "/oss-weather/", key, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "e2e", "version": "1"}})
    code, text, _ = mcp("gw1", "/oss-weather/", key, "tools/call", {"name": "get-forecast", "arguments": {"city": "Oslo"}}, sid)
    check("6.blocked tool refused", code >= 400 or '"error"' in text, (code, text[:200]))
    code, text, _ = mcp("gw1", "/oss-weather/", key, "tools/call", {"name": "get-weather", "arguments": {"city": "Oslo"}}, sid)
    check("6.allowed tool works", code == 200 and "Oslo" in text, (code, text[:200]))
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("6.credential not drifting", cr.get("drift") == "none", cr.get("drift"))

    # ---------- 7. a third node joins ----------
    up = subprocess.run(COMPOSE + ["--profile", "scale", "up", "-d", "gw3"], capture_output=True, text=True)
    check("7.gw3 started", up.returncode == 0, up.stderr[-300:])
    for _ in range(30):
        if http("GET", GW["gw3"] + "/hello", raw=True)[0] == 200:
            break
        time.sleep(1)
    clean_gateways(("gw3",))
    lock = studio("GET", f"/api/v1/tyk-connections/{cid}")[1]["lock_version"]
    code, _ = studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"gateway_node_urls": [GW["gw2"], GW["gw3"]], "lock_version": lock})
    check("7.add gw3 to the node list", code == 200, code)
    check("7.gw3 starts without the Studio proxy", api_id not in (node_ids("gw3") or []))
    code, run = sync(cid)
    check("7.sync fills gw3", api_id in (node_ids("gw3") or []), run)
    check("7.but not with the operator's proxies", "ops-weather" not in (node_ids("gw3") or []))
    code, names = tools("gw3", "/oss-weather/", key)
    check("7.existing key works on the new node (shared Redis)", code == 200 and names == ["get-weather"], (code, names))
    code, nodes = studio("GET", f"/api/v1/tyk-connections/{cid}/nodes")
    check("7.three nodes listed", len(nodes) == 3, [n["address"] for n in nodes])

    # ---------- 8. an edit behind Studio's back is undone ----------
    code, doc = gw("gw2", "GET", "/tyk/mcps/" + api_id)
    doc["x-tyk-api-gateway"]["upstream"]["url"] = "http://host.docker.internal:1"
    code, _ = gw("gw2", "PUT", "/tyk/mcps/" + api_id, doc)
    gw("gw2", "GET", "/tyk/reload?block=true")
    code, run = sync(cid)
    code, doc = gw("gw2", "GET", "/tyk/mcps/" + api_id)
    check("8.tampered proxy restored", doc["x-tyk-api-gateway"]["upstream"]["url"] == UPSTREAM, doc["x-tyk-api-gateway"]["upstream"])

    # ---------- 9. a node goes down ----------
    subprocess.run(COMPOSE + ["--profile", "scale", "stop", "gw3"], capture_output=True)
    code, run = sync(cid)
    check("9.sync with a node down is partial", run.get("status") == "partial", run.get("status"))
    code, nodes = studio("GET", f"/api/v1/tyk-connections/{cid}/nodes")
    states = {n["address"]: n["state"] for n in nodes}
    check("9.gw3 unreachable", states.get(GW["gw3"]) == "unreachable", states)
    code, names = tools("gw1", "/oss-weather/", key)
    check("9.the others keep serving", code == 200 and names, (code, names))
    lock = studio("GET", f"/api/v1/tyk-connections/{cid}")[1]["lock_version"]
    studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"gateway_node_urls": [GW["gw2"]], "lock_version": lock})
    code, run = sync(cid)
    code, nodes = studio("GET", f"/api/v1/tyk-connections/{cid}/nodes")
    states = {n["address"]: n["state"] for n in nodes}
    check("9.removed node is marked gone", states.get(GW["gw3"]) == "gone", states)

    # ---------- 10. rotate and revoke ----------
    code, rot = studio("POST", f"/api/v1/mcp-credentials/{cred['id']}/rotate")
    check("10.rotate", code in (200, 201) and rot.get("key"), (code, "no key"))
    new_key, new_cred = rot.get("key", ""), rot.get("credential", {})
    code, _ = tools("gw2", "/oss-weather/", key)
    check("10.old key refused", code in (401, 403), code)
    code, names = tools("gw2", "/oss-weather/", new_key)
    check("10.new key works", code == 200 and names == ["get-weather"], (code, names))
    code, rev = studio("POST", f"/api/v1/mcp-credentials/{new_cred.get('id')}/revoke", {"reason": "e2e"})
    check("10.revoke", code == 200 and rev.get("status") == "revoked" and rev.get("revoke_mode") == "deleted", (code, rev))
    code, _ = tools("gw1", "/oss-weather/", new_key)
    check("10.revoked key refused", code in (401, 403), code)

    # ---------- 11. guardrails ----------
    code, err = studio("DELETE", f"/api/v1/mcp-servers/{w['id']}")
    check("11.operator proxy cannot be deleted from Studio", code >= 400, (code, err))
    code, err = studio("POST", f"/api/v1/tyk-connections/{cid}/policies", {"kind": "access", "name": "x", "server_id": srv["id"]})
    check("11.no policies on a gateway connection", code >= 400, (code, err))
    code, err = studio("GET", f"/api/v1/tyk-connections/{cid}/apis")
    check("11.no REST API listing on a gateway connection", code >= 400, (code, err))

    # ---------- 12. delete the Studio proxy ----------
    code, _ = studio("DELETE", f"/api/v1/mcp-servers/{srv['id']}?force=true")
    check("12.delete the Studio proxy", code in (200, 204), code)
    check("12.gone from gw1 and gw2", api_id not in (node_ids("gw1") or []) and api_id not in (node_ids("gw2") or []))
    check("12.operator proxy untouched", "ops-weather" in (node_ids("gw1") or []) and "ops-weather" in (node_ids("gw2") or []))

    # ---------- 13. DNS discovery (localhost resolves to every loopback address) ----------
    code, dconn = studio("POST", "/api/v1/tyk-connections", {
        "name": "OSS DNS", "kind": "gateway", "dashboard_url": GW["gw1"], "dashboard_access_token": GW_SECRET,
        "declared_mode": "catalogue", "allow_internal_host": True, "gateway_discovery": "dns"})
    check("13.create dns connection", code == 201, (code, dconn))
    if code == 201:
        created["connections"].append(dconn["id"])
        code, act = studio("POST", f"/api/v1/tyk-connections/{dconn['id']}/activate")
        check("13.activate dns connection", code == 200, (code, act))
        sync(dconn["id"])
        code, nodes = studio("GET", f"/api/v1/tyk-connections/{dconn['id']}/nodes")
        check("13.nodes resolved and dialled by address", code == 200 and len(nodes) >= 1 and all(n.get("pinned_ip") and n["reachable"] for n in nodes),
              [(n["address"], n["reachable"]) for n in nodes or []])


def cleanup():
    for a in created["apps"]:
        studio("DELETE", f"/api/v1/apps/{a}")
    for c in created["connections"]:
        studio("DELETE", f"/api/v1/tyk-connections/{c}?force=true")
    for c in created["catalogues"]:
        studio("DELETE", f"/api/v1/tool-catalogues/{c}")
    subprocess.run(COMPOSE + ["--profile", "scale", "stop", "gw3"], capture_output=True)
    clean_gateways(("gw1", "gw2"))


if __name__ == "__main__":
    try:
        main()
    finally:
        cleanup()
    print(f"\n{len(failures)} failure(s)" + (": " + ", ".join(failures) if failures else ""))
    sys.exit(1 if failures else 0)
