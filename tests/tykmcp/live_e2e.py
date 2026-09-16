"""Live verification of features/TykMCPIntegration.md section 16 against a
real Tyk Dashboard + Gateway and an enterprise AI Studio.

Environment (all required unless noted):
  STUDIO_URL           AI Studio API, e.g. http://localhost:8090
  STUDIO_API_KEY       an administrator's API key
  TYK_DASHBOARD_URL    the Dashboard as reached from this machine, e.g. http://localhost:3000
  TYK_DASHBOARD_URL_FROM_STUDIO  the Dashboard as reached from the Studio process
                       (default http://host.docker.internal:3000 for the dev stack)
  TYK_DASHBOARD_KEY    a Dashboard user access key with API, policy, MCP and key write
  TYK_GATEWAY_URL      the Gateway, e.g. http://localhost:8080
  MCP_UPSTREAM_URL     the demo MCP server as reached from the Gateway
                       (run mcpsrv/main.go on the host: http://host.docker.internal:4020)
  MCP_UPSTREAM_LOG     path of the demo server's log (asserts the static header arrived)
  WEBHOOK_URL          a receiver as reached from Studio (run hookrecv.py 4021:
                       http://host.docker.internal:4021/hook)
  WEBHOOK_LOG          hookrecv.py's hooks.jsonl

Keys are never printed. Every step prints PASS/FAIL; everything created is
removed at the end (best effort). Exit code 1 on any failure.
"""
import json
import os
import subprocess
import tempfile
import sys
import time
import traceback


def env(name, default=None):
    v = os.environ.get(name, default)
    if v is None:
        print("missing environment variable", name)
        sys.exit(2)
    return v


STUDIO = env("STUDIO_URL", "http://localhost:8090").rstrip("/")
DASH = env("TYK_DASHBOARD_URL", "http://localhost:3000").rstrip("/")
DASH_FROM_STUDIO = env("TYK_DASHBOARD_URL_FROM_STUDIO", "http://host.docker.internal:3000").rstrip("/")
GW = env("TYK_GATEWAY_URL", "http://localhost:8080").rstrip("/")
SKEY = env("STUDIO_API_KEY")
DKEY = env("TYK_DASHBOARD_KEY")
UPSTREAM = env("MCP_UPSTREAM_URL", "http://host.docker.internal:4020")
UPSTREAM_LOG = env("MCP_UPSTREAM_LOG", os.path.join(os.path.dirname(os.path.abspath(__file__)), "mcpsrv", "mcpsrv.log"))
WEBHOOK_URL = env("WEBHOOK_URL", "http://host.docker.internal:4021/hook")
HOOKS = env("WEBHOOK_LOG", os.path.join(os.path.dirname(os.path.abspath(__file__)), "hooks.jsonl"))
SP = os.path.dirname(os.path.abspath(__file__))
UPSTREAM_SECRET = "demo-upstream-secret-" + str(int(time.time()))

failures = []
created = {"servers": [], "dash_policies": [], "dash_mcps": [], "dash_assets": [], "catalogues": [], "apps": [], "targets": [], "conn": None}
TEMPLATE_ID = "ai-studio-e2e-template"


def http(method, url, body=None, headers=None, raw=False):
    hfile = os.path.join(tempfile.gettempdir(), f"tykmcp_headers_{os.getpid()}.tmp")
    args = ["curl", "-s", "-X", method, url, "-H", "Content-Type: application/json", "-w", "\n%{http_code}", "-D", hfile]
    for k, v in (headers or {}).items():
        args += ["-H", f"{k}: {v}"]
    if body is not None:
        args += ["--data-binary", json.dumps(body)]
    out = subprocess.run(args, capture_output=True, text=True).stdout
    text, _, code = out.rpartition("\n")
    hdrs = {}
    try:
        for line in open(hfile).read().split("\n")[1:]:
            if ":" in line:
                k, v = line.split(":", 1)
                hdrs[k.strip().lower()] = v.strip()
    except OSError:
        pass
    if not code.strip().isdigit():
        code = "0"
    if raw:
        return int(code), text, hdrs
    try:
        return int(code), json.loads(text), hdrs
    except Exception:
        return int(code), text[:500], hdrs


def studio(method, path, body=None):
    code, data, _ = http(method, STUDIO + path, body, {"Authorization": "Bearer " + SKEY})
    return code, data


def dash(method, path, body=None):
    code, data, _ = http(method, DASH + path, body, {"Authorization": DKEY})
    return code, data


def check(name, ok, detail=""):
    print(("PASS " if ok else "FAIL ") + name + (("  -- " + str(detail)[:300]) if detail else ""))
    if not ok:
        failures.append(name)
    return ok


def sync(conn_id):
    code, run = studio("POST", f"/api/v1/tyk-connections/{conn_id}/sync?wait=true")
    return code, run


def wait_gateway_policies(seconds=12):
    # The Gateway picks up new policies and proxies from the Dashboard on its
    # own reload cycle; writes that reference them race it.
    time.sleep(seconds)


def wait_gateway_path(path, seconds=150):
    deadline = time.time() + seconds
    while time.time() < deadline:
        code, _, _ = http("POST", GW + path, {}, {"Content-Type": "application/json"}, raw=True)
        if code != 404:
            return True
        time.sleep(3)
    return False


def mcp_call(key, path, method_name, params, session=None):
    hdrs = {"Accept": "application/json, text/event-stream", "Content-Type": "application/json"}
    if key:
        hdrs["Authorization"] = key
    if session:
        hdrs["Mcp-Session-Id"] = session
    body = {"jsonrpc": "2.0", "id": 1, "method": method_name, "params": params}
    code, text, h = http("POST", GW + path, body, hdrs, raw=True)
    return code, text, h.get("mcp-session-id")


try:
    # ---------- 1. connection ----------
    code, st = studio("GET", "/api/v1/tyk-mcp/status")
    check("1.status available+enabled", code == 200 and st.get("available") and st.get("enabled"), st)
    code, conn = studio("POST", "/api/v1/tyk-connections", {
        "name": "Live Tyk 5.14", "dashboard_url": DASH_FROM_STUDIO, "dashboard_access_token": DKEY,
        "declared_mode": "full", "allow_internal_host": True, "gateway_base_url": GW,
        "known_gateway_tags": [{"tag": "edge-eu", "label": "EU edge"}], "sync_interval_seconds": 300,
    })
    check("1.create connection", code == 201, (code, conn if code != 201 else conn.get("status")))
    created["conn"] = conn["id"]
    cid = conn["id"]
    code, act = studio("POST", f"/api/v1/tyk-connections/{cid}/activate")
    caps = act.get("capabilities", {}) if isinstance(act, dict) else {}
    check("1.activate -> active", code == 200 and act.get("status") == "active", (code, act if code != 200 else act.get("effective_mode")))
    check("1.probe mcp_supported=ok", caps.get("mcp_supported", {}).get("state") == "ok", caps.get("mcp_supported"))
    check("1.probe rest_to_mcp=no (5.14)", caps.get("rest_to_mcp_supported", {}).get("state") == "no", caps.get("rest_to_mcp_supported"))
    check("1.probe: mcp_write permitted, other writes unverified", caps.get("mcp_write", {}).get("state") == "ok" and all(caps.get(k, {}).get("state") == "unverified" for k in ("policies_write", "keys_write")), {k: caps.get(k, {}).get("state") for k in ("mcp_write", "policies_write", "keys_write")})
    check("1.effective mode full", act.get("effective_mode") == "full", act.get("effective_mode"))

    # ---------- 2. register a remote proxy (with the connection's API template) ----------
    dash("DELETE", "/api/assets/" + TEMPLATE_ID)
    code, tpl = dash("POST", "/api/assets", {
        "id": TEMPLATE_ID, "kind": "oas-template", "name": "AI Studio e2e governance", "description": "traffic logs on every proxy Studio creates",
        "data": {"openapi": "3.0.3", "info": {"title": "template", "version": "1.0.0"}, "paths": {},
                 "x-tyk-api-gateway": {"info": {"name": "template", "state": {"active": True}}, "server": {"listenPath": {"value": "/template/", "strip": True}},
                                       "upstream": {"url": "http://template.invalid"},
                                       "middleware": {"global": {"trafficLogs": {"enabled": True}, "contextVariables": {"enabled": True}}}}},
    })
    check("2.template asset created on Dashboard", code in (200, 201), (code, tpl))
    created["dash_assets"].append(TEMPLATE_ID)
    code, cc = studio("GET", f"/api/v1/tyk-connections/{cid}")
    code, upd = studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"template_id": TEMPLATE_ID, "lock_version": cc.get("lock_version")})
    check("2.template id saved and probed", code == 200 and upd.get("template_id") == TEMPLATE_ID and upd.get("capabilities", {}).get("template_read", {}).get("state") == "ok", (code, upd.get("template_id"), upd.get("capabilities", {}).get("template_read")))
    reg = {
        "connection_id": cid, "kind": "remote", "name": "Live Weather MCP", "listen_path": "/live-weather/",
        "upstream_url": UPSTREAM, "upstream_auth_header_name": "X-Upstream-Token", "upstream_auth_token": UPSTREAM_SECRET,
        "consumer_auth": "auth_token", "gateway_tags": [], "confirm_no_gateway_tags": True, "privacy_score": 20,
        "description": "Live weather demo behind Tyk",
    }
    code, prev = studio("POST", "/api/v1/mcp-servers/register?dry_run=1", reg)
    check("2.preview rendered locally", code == 200 and prev.get("endpoint_url") == GW + "/live-weather/mcp" and not prev.get("dashboard_validated"), (code, prev))
    check("2.dry run preview masked", UPSTREAM_SECRET not in json.dumps(prev), "")
    code, res = studio("POST", "/api/v1/mcp-servers/register", reg)
    check("2.create proxy", code == 201, (code, res))
    srv = res["server"]
    created["servers"].append(srv["id"])
    check("2.origin studio, active on Dashboard", srv["origin"] == "studio" and srv["dashboard_state"] == "active", (srv["origin"], srv["dashboard_state"]))
    check("2.studio never shows the secret", UPSTREAM_SECRET not in json.dumps(res), "")
    code, live = dash("GET", "/api/mcps/" + srv["tyk_api_id"])
    check("2.proxy exists on Dashboard", code == 200 and live.get("x-tyk-api-gateway", {}).get("server", {}).get("listenPath", {}).get("value") == "/live-weather/", code)
    glob = live.get("x-tyk-api-gateway", {}).get("middleware", {}).get("global", {})
    hdr = glob.get("transformRequestHeaders", {})
    check("2.Dashboard holds the upstream header", any(h.get("value") == UPSTREAM_SECRET for h in hdr.get("add", [])), hdr)
    check("2.template defaults merged into the proxy", glob.get("trafficLogs", {}).get("enabled") is True and glob.get("contextVariables", {}).get("enabled") is True, glob)
    check("2.registration values win over the template", live.get("x-tyk-api-gateway", {}).get("upstream", {}).get("url", "").startswith(UPSTREAM.rstrip("/")), live.get("x-tyk-api-gateway", {}).get("upstream"))
    check("2.preview mentions the template", any("template" in w.lower() for w in prev.get("warnings", [])), prev.get("warnings"))
    code, c2 = studio("GET", f"/api/v1/tyk-connections/{cid}")
    check("2.capability mcp_write now ok", c2.get("capabilities", {}).get("mcp_write", {}).get("state") == "ok", c2.get("capabilities", {}).get("mcp_write"))

    # ---------- 3. policies via the creator, pin, publish, grant ----------
    code, acl = studio("POST", f"/api/v1/tyk-connections/{cid}/policies", {"kind": "access", "name": "Live weather access", "server_id": srv["id"], "pin": True})
    check("3.create access policy", code == 201 and acl.get("studio_managed"), (code, acl))
    created["dash_policies"].append(acl.get("tyk_policy_id"))
    code, gold = studio("POST", f"/api/v1/tyk-connections/{cid}/policies", {"kind": "consumption", "name": "Live gold plan", "server_id": srv["id"], "rate": 100, "per": 60, "pin": True})
    check("3.create consumption policy", code == 201, (code, gold))
    created["dash_policies"].append(gold.get("tyk_policy_id"))
    code, silver = studio("POST", f"/api/v1/tyk-connections/{cid}/policies", {"kind": "consumption", "name": "Live silver plan", "server_id": srv["id"], "rate": 50, "per": 60, "pin": False})
    check("3.create second consumption policy (unpinned)", code == 201, code)
    created["dash_policies"].append(silver.get("tyk_policy_id"))
    wait_gateway_policies()
    code, c3 = studio("GET", f"/api/v1/tyk-connections/{cid}")
    check("3.capability policies_write ok", c3.get("capabilities", {}).get("policies_write", {}).get("state") == "ok", {k: v.get("state") for k, v in c3.get("capabilities", {}).items()})
    code, detail = studio("GET", f"/api/v1/mcp-servers/{srv['id']}")
    check("3.bundle pinned + brokerable", len(detail.get("bundle", [])) == 2 and detail.get("brokerable"), (len(detail.get("bundle", [])), detail.get("brokerable")))
    code, pub = studio("POST", f"/api/v1/mcp-servers/{srv['id']}/activate")
    check("3.publish", code == 200 and pub.get("is_active"), (code, pub))
    # Portal visibility goes through tool catalogues (Catalogs -> Teams), like
    # tools: put the server in a catalogue every team is granted.
    code, cats = studio("GET", "/api/v1/tool-catalogues?all=true")
    clist = cats.get("data") if isinstance(cats, dict) else cats
    cat_id = next((int(c["id"]) for c in (clist or []) if (c.get("attributes") or {}).get("name") == "Live MCP e2e"), None)
    if cat_id is None:
        code, cat = studio("POST", "/api/v1/tool-catalogues", {"data": {"type": "ToolCatalogue", "attributes": {"name": "Live MCP e2e", "short_description": "e2e"}}})
        cat_id = int(cat["data"]["id"])
        created["catalogues"].append(cat_id)
    code, groups = studio("GET", "/api/v1/groups")
    glist = groups if isinstance(groups, list) else groups.get("data") or groups.get("groups") or []
    gids = [int(g["id"]) for g in glist]
    for g in gids:
        studio("POST", f"/api/v1/groups/{g}/tool-catalogues", {"data": {"type": "ToolCatalogue", "id": str(cat_id)}})
    code, _ = studio("PUT", f"/api/v1/mcp-servers/{srv['id']}/catalogues", {"tool_catalogue_ids": [cat_id]})
    check("3.attach to catalogue", code == 200, (code, cat_id))
    code, bad = studio("PUT", f"/api/v1/mcp-servers/{srv['id']}/catalogues", {"tool_catalogue_ids": ["1"]})
    check("3.string catalogue ids are refused", code == 400, code)
    code, detail = studio("GET", f"/api/v1/mcp-servers/{srv['id']}")
    check("3.server lists its catalogue by name", any(c.get("name") == "Live MCP e2e" for c in detail.get("tool_catalogues", [])), detail.get("tool_catalogues"))

    # ---------- 4. portal: App, approval, key, gateway call ----------
    code, items = studio("GET", "/common/catalog?type=mcp_server")
    found = [i for i in (items.get("data") or items.get("items") or []) if str(i.get("id")) == str(srv["id"]) or i.get("attributes", {}).get("name") == "Live Weather MCP"] if isinstance(items, dict) else []
    check("4.portal catalogue lists the server", len(found) >= 1, (code, str(items)[:200]))
    code, app = studio("POST", "/common/apps", {"name": "Live weather app", "description": "e2e", "data_source_ids": [], "llm_ids": [], "tool_ids": [], "mcp_server_ids": [srv["id"]]})
    check("4.create App with the server", code == 201, (code, app))
    app_id = app["id"] if isinstance(app, dict) else None
    created["apps"].append(app_id)
    code, summ = studio("GET", f"/common/apps/{app_id}/mcp")
    check("4.before approval: cannot mint", code == 200 and summ["connections"] and not summ["connections"][0]["can_mint"], summ)
    code, _ = studio("POST", f"/api/v1/apps/{app_id}/activate-credential")
    check("4.approve App", code in (200, 204), code)
    code, minted = studio("POST", f"/common/apps/{app_id}/mcp/credentials", {"connection_id": cid})
    check("4.mint key", code == 201 and minted.get("key"), (code, minted if code != 201 else "ok"))
    key = minted.get("key", "")
    cred = minted.get("credential", {})
    check("4.key covers the server at the gateway URL", minted.get("servers", [{}])[0].get("endpoint_url") == GW + "/live-weather/mcp", minted.get("servers"))
    check("4.applied policies = bundle", sorted(cred.get("applied_policy_ids", [])) == sorted([acl["tyk_policy_id"], gold["tyk_policy_id"]]), cred.get("applied_policy_ids"))
    code, c4 = studio("GET", f"/api/v1/tyk-connections/{cid}")
    check("4.capability keys_write ok", c4.get("capabilities", {}).get("keys_write", {}).get("state") == "ok", "")
    wait_gateway_policies()
    if not check("4.gateway loaded the proxy", wait_gateway_path("/live-weather/mcp"), ""):
        print("   (inspect the Gateway log for reload and listen path messages)")
    code, text, sess = mcp_call(key, "/live-weather/mcp", "initialize", {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "e2e", "version": "1"}})
    check("4.gateway initialize with key -> 200", code == 200 and "weather-demo" in text, (code, text[:200]))
    code2, text2, _ = mcp_call(key, "/live-weather/mcp", "tools/list", {}, sess)
    check("4.gateway tools/list", code2 == 200 and "get-weather" in text2, (code2, text2[:200]))
    code3, text3, _ = mcp_call("", "/live-weather/mcp", "initialize", {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "e2e", "version": "1"}})
    check("4.gateway refuses without key", code3 in (401, 403), (code3, text3[:120]))
    log = open(UPSTREAM_LOG).read()
    check("4.upstream saw the static header", f'x-upstream-token="{UPSTREAM_SECRET}"' in log, log[-200:])
    code, dk = dash("GET", f"/api/keys/{cred['tyk_key_hash']}?hashed=true")
    data = dk.get("data", {}) if isinstance(dk, dict) else {}
    check("4.Dashboard key carries both policies", sorted(data.get("apply_policies", [])) == sorted([acl["tyk_policy_id"], gold["tyk_policy_id"]]), data.get("apply_policies"))
    check("4.Dashboard key meta_data.studio_app_id + tag", str(data.get("meta_data", {}).get("studio_app_id")) == str(app_id) and "ai-studio" in data.get("tags", []), (data.get("meta_data"), data.get("tags")))
    check("4.expires matches ledger", (cred.get("expires_at") is None and data.get("expires") in (0, None)) or (cred.get("expires_at") is not None and data.get("expires")), (cred.get("expires_at"), data.get("expires")))
    code, rep = studio("GET", "/api/v1/mcp-access-report")
    check("4.access report row", code == 200 and any(r.get("credential_id") == cred.get("id") for r in rep), (code, len(rep) if isinstance(rep, list) else rep))

    # ---------- 5. drift ----------
    code, gpol = dash("GET", "/api/portal/policies/" + gold["tyk_policy_id"])
    gpol["rate"] = 200
    code, _ = dash("PUT", "/api/portal/policies/" + gold["tyk_policy_id"], gpol)
    check("5.edit consumption rate on Dashboard", code == 200, code)
    code, run = sync(cid)
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("5.no drift after a Dashboard-side limit edit", cr.get("drift") == "none" and cr.get("status") == "active", (cr.get("drift"), cr.get("status")))
    code, _ = studio("PUT", f"/api/v1/mcp-servers/{srv['id']}/bundle", {"pins": [{"tyk_policy_id": acl["tyk_policy_id"], "role": "access"}, {"tyk_policy_id": silver["tyk_policy_id"], "role": "consumption"}]})
    check("5.re-pin consumption -> silver", code == 200, code)
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("5.narrowing applied automatically", sorted(cr.get("applied_policy_ids", [])) == sorted([acl["tyk_policy_id"], silver["tyk_policy_id"]]) and cr.get("drift") == "none", (cr.get("applied_policy_ids"), cr.get("drift")))
    code, dk = dash("GET", f"/api/keys/{cred['tyk_key_hash']}?hashed=true")
    check("5.Dashboard key updated", silver["tyk_policy_id"] in dk.get("data", {}).get("apply_policies", []) and gold["tyk_policy_id"] not in dk.get("data", {}).get("apply_policies", []), dk.get("data", {}).get("apply_policies"))
    # second server, bound to the App -> widening
    reg2 = dict(reg)
    reg2.update({"name": "Live Tickets MCP", "listen_path": "/live-tickets/", "description": "second live server"})
    code, res2 = studio("POST", "/api/v1/mcp-servers/register", reg2)
    check("5.register second server", code == 201, (code, res2))
    srv2 = res2["server"]
    created["servers"].append(srv2["id"])
    code, acl2 = studio("POST", f"/api/v1/tyk-connections/{cid}/policies", {"kind": "access", "name": "Live tickets access", "server_id": srv2["id"], "pin": True})
    created["dash_policies"].append(acl2.get("tyk_policy_id"))
    wait_gateway_policies()
    studio("POST", f"/api/v1/mcp-servers/{srv2['id']}/activate")
    studio("PUT", f"/api/v1/mcp-servers/{srv2['id']}/catalogues", {"tool_catalogue_ids": [cat_id]})
    code, upd = studio("PATCH", f"/api/v1/apps/{app_id}", {"data": {"type": "apps", "attributes": {"name": "Live weather app", "description": "e2e", "user_id": app["attributes"]["user_id"], "datasource_ids": [], "llm_ids": [], "tool_ids": [], "mcp_server_ids": [srv["id"], srv2["id"]]}}})
    check("5.bind second server to App", code == 200, (code, upd))
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("5.widening waits for approval", cr.get("drift") == "pending_widen", (cr.get("drift"), cr.get("desired_policy_ids")))
    code, cr = studio("POST", f"/api/v1/mcp-credentials/{cred['id']}/apply-drift")
    check("5.apply widening", code == 200 and cr.get("drift") == "none" and acl2["tyk_policy_id"] in cr.get("applied_policy_ids", []), (code, cr.get("drift"), cr.get("applied_policy_ids")))
    # foreign policy added on the Dashboard
    code, foreign = dash("POST", "/api/portal/policies", {"name": "live-foreign-plan", "active": True, "partitions": {"acl": False, "rate_limit": True, "quota": False, "complexity": False, "per_api": False}, "rate": 7, "per": 60, "quota_max": -1, "quota_renewal_rate": -1, "throttle_interval": -1, "throttle_retry_limit": -1, "access_rights": {}, "tags": ["e2e"]})
    fid = foreign.get("Message")
    created["dash_policies"].append(fid)
    wait_gateway_policies()
    code, dk = dash("GET", f"/api/keys/{cred['tyk_key_hash']}?hashed=true")
    kdata = dk["data"]
    kdata["apply_policies"] = kdata.get("apply_policies", []) + [fid]
    code, _ = dash("PUT", f"/api/keys/{cred['tyk_key_hash']}?hashed=true", kdata)
    check("5.add foreign policy on Dashboard", code == 200, code)
    sync(cid)
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("5.foreign policy reported as external, kept", fid in cr.get("external_policy_ids", []) and cr.get("drift") == "none", (cr.get("external_policy_ids"), cr.get("drift")))
    code, dk = dash("GET", f"/api/keys/{cred['tyk_key_hash']}?hashed=true")
    check("5.foreign policy still on the key", fid in dk["data"].get("apply_policies", []), dk["data"].get("apply_policies"))

    # ---------- 6. missing / resume / deleted key ----------
    code, before = dash("GET", "/api/mcps/" + srv["tyk_api_id"])
    code, _ = dash("DELETE", "/api/mcps/" + srv["tyk_api_id"])
    check("6.delete proxy on Dashboard", code == 200, code)
    sync(cid)
    code, s6 = studio("GET", f"/api/v1/mcp-servers/{srv['id']}")
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("6.server missing + unpublished", s6.get("dashboard_state") == "missing" and not s6.get("is_active"), (s6.get("dashboard_state"), s6.get("is_active")))
    if not check("6.key narrowed to remaining server (still active) or suspended", cr.get("status") in ("active", "suspended") and acl["tyk_policy_id"] not in cr.get("applied_policy_ids", []), (cr.get("status"), cr.get("applied_policy_ids"))):
        print("   credential:", json.dumps({k: cr.get(k) for k in ("status", "drift", "drift_detail", "last_error", "applied_policy_ids", "desired_policy_ids", "external_policy_ids")}))
        print("   (inspect the Studio log for drift evaluation warnings)")
    before.pop("x-tyk-api-gateway", None) if False else None
    code, rec = dash("POST", "/api/mcps", before)
    check("6.recreate proxy under the same id", code == 200 and rec.get("ID") == srv["tyk_api_id"], (code, rec))
    if code == 200 and rec.get("ID") != srv["tyk_api_id"]:
        created["dash_mcps"].append(rec.get("ID"))
    sync(cid)
    code, s6 = studio("GET", f"/api/v1/mcp-servers/{srv['id']}")
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("6.server resumed", s6.get("dashboard_state") == "active", s6.get("dashboard_state"))
    check("6.key restored without approval", cr.get("status") == "active" and acl["tyk_policy_id"] in cr.get("applied_policy_ids", []) and cr.get("drift") == "none", (cr.get("status"), cr.get("applied_policy_ids"), cr.get("drift")))
    code, _ = dash("DELETE", f"/api/keys/{cred['tyk_key_hash']}?hashed=true")
    check("6.delete key on Dashboard", code == 200, code)
    sync(cid)
    code, cr = studio("GET", f"/api/v1/mcp-credentials/{cred['id']}")
    check("6.ledger shows revoked/deleted_on_dashboard", cr.get("status") == "revoked" and "deleted" in (cr.get("revoke_reason") or ""), (cr.get("status"), cr.get("revoke_reason")))
    # a fresh key for later cleanup coverage: rotate path
    code, minted2 = studio("POST", f"/common/apps/{app_id}/mcp/credentials", {"connection_id": cid})
    check("6.owner can mint again", code == 201, (code, minted2 if code != 201 else "ok"))
    cred2 = minted2.get("credential", {})
    code, rot = studio("POST", f"/api/v1/mcp-credentials/{cred2['id']}/rotate")
    check("6.admin rotate", code == 201 and rot.get("key") and rot["key"] != minted2.get("key"), code)
    cred3 = rot.get("credential", {})

    # ---------- 7. catalogue mode, submission, handoff, webhook, link ----------
    code, tgt = studio("POST", "/api/v1/webhooks/targets", {"name": "e2e platform team", "url": WEBHOOK_URL, "topic_filters": ["system.mcp_server.*"], "template_preset": "standard"})
    check("7.create webhook target", code == 201, (code, tgt))
    tid = tgt.get("target", {}).get("id")
    created["targets"].append(tid)
    code, _ = studio("POST", f"/api/v1/webhooks/targets/{tid}/approve", {"note": "e2e"})
    check("7.approve webhook target", code == 200, code)
    code, c7 = studio("GET", f"/api/v1/tyk-connections/{cid}")
    code, c7 = studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"declared_mode": "catalogue", "lock_version": c7.get("lock_version")})
    check("7.switch to catalogue", code == 200 and c7.get("effective_mode") == "catalogue", (code, c7 if code != 200 else c7.get("effective_mode")))
    code, m7 = studio("POST", f"/api/v1/mcp-credentials", {"app_id": int(app_id), "connection_id": cid})
    check("7.mint refused in catalogue mode", code in (403, 409, 422), (code, m7))
    if os.path.exists(HOOKS):
        os.remove(HOOKS)
    code, sub = studio("POST", "/common/submissions", {"data": {"attributes": {
        "resource_type": "mcp_server", "status": "submitted",
        "resource_payload": {"name": "Live Community MCP", "description": "submitted by a portal user", "kind": "remote", "connection_id": cid,
                             "upstream_url": UPSTREAM, "upstream_auth_header_name": "X-Upstream-Token", "upstream_auth_token": UPSTREAM_SECRET,
                             "consumer_auth": "auth_token", "suggested_listen_path": "/live-community/", "gateway_tags": ["edge-eu"]},
        "suggested_privacy": 30, "privacy_justification": "public demo data", "primary_contact": "ux_admin@tyk.io"}}})
    check("7.create submission", code == 201, (code, sub))
    sid = sub.get("data", {}).get("id")
    check("7.submission response redacts the credential", UPSTREAM_SECRET not in json.dumps(sub), "")
    code, t7 = studio("POST", f"/api/v1/submissions/{sid}/test")
    check("7.test is skipped on a catalogue connection", code == 200 and t7.get("data", {}).get("skipped"), (code, t7))
    code, ap = studio("POST", f"/api/v1/submissions/{sid}/approve", {"data": {"attributes": {"final_privacy_score": 30, "review_notes": "e2e"}}})
    check("7.approve -> handoff", code == 200 and ap.get("data", {}).get("resource_id"), (code, ap))
    pending_id = ap.get("data", {}).get("resource_id")
    created["servers"].append(pending_id)
    code, p7 = studio("GET", f"/api/v1/mcp-servers/{pending_id}")
    check("7.server awaiting the platform team", p7.get("dashboard_state") == "pending_platform", p7.get("dashboard_state"))
    studio("PUT", f"/api/v1/mcp-servers/{pending_id}/catalogues", {"tool_catalogue_ids": [cat_id]})
    code, pkg = studio("GET", f"/api/v1/mcp-servers/{pending_id}/handoff")
    check("7.handoff package (masked)", code == 200 and UPSTREAM_SECRET not in json.dumps(pkg) and '"***"' in json.dumps(pkg), code)
    code, pkgs = studio("GET", f"/api/v1/mcp-servers/{pending_id}/handoff?include_secrets=true")
    check("7.handoff package with credential", code == 200 and UPSTREAM_SECRET in json.dumps(pkgs), code)
    deadline = time.time() + 15
    hook = None
    while time.time() < deadline and hook is None:
        if os.path.exists(HOOKS):
            for line in open(HOOKS):
                e = json.loads(line)
                if e.get("topic") == "system.mcp_server.registration_handoff":
                    hook = e
        time.sleep(0.5)
    check("7.webhook delivered registration_handoff", hook is not None and "Live Community MCP" in hook["body"] and UPSTREAM_SECRET not in hook["body"], (hook or {}).get("body", "")[:200])
    # platform team creates it by hand from the package
    code, made = dash("POST", "/api/mcps", json.loads(json.dumps(pkgs["definition"])))
    check("7.platform team creates the proxy from the package", code == 200 and made.get("ID"), (code, made))
    created["dash_mcps"].append(made.get("ID"))
    sync(cid)
    code, pkg = studio("GET", f"/api/v1/mcp-servers/{pending_id}/handoff")
    check("7.candidate found after sync", any(c.get("tyk_api_id") == made.get("ID") for c in pkg.get("candidates", [])), [c.get("tyk_api_id") for c in pkg.get("candidates", [])])
    code, linked = studio("POST", f"/api/v1/mcp-servers/{pending_id}/link", {"tyk_api_id": made.get("ID")})
    check("7.link", code == 200 and linked.get("tyk_api_id") == made.get("ID") and linked.get("origin") == "submission" and linked.get("dashboard_state") == "active", (code, linked))
    if code == 200:
        created["servers"].remove(pending_id)
        created["servers"].append(linked["id"])
    code, live8 = dash("GET", "/api/mcps/" + made.get("ID"))
    check("8.gatewayTags on the Dashboard definition", live8.get("x-tyk-api-gateway", {}).get("server", {}).get("gatewayTags", {}) == {"enabled": True, "tags": ["edge-eu"]}, live8.get("x-tyk-api-gateway", {}).get("server", {}).get("gatewayTags"))
    check("8.tags on the catalogue row", linked.get("gateway_tags", {}).get("tags") == ["edge-eu"], linked.get("gateway_tags"))
    code, pub8 = studio("POST", f"/api/v1/mcp-servers/{linked['id']}/activate")
    check("8.publish the linked server", code == 200 and pub8.get("is_active"), (code, pub8))
    code, pd = studio("GET", f"/common/catalog/mcp-servers/{linked['id']}")
    check("8.portal detail shows the tags", code == 200 and "edge-eu" in json.dumps(pd), (code, str(pd)[:160]))

except Exception:
    traceback.print_exc()
    failures.append("exception")
finally:
    print("---- cleanup ----")
    if created["conn"]:
        cid = created["conn"]
        c, cc = studio("GET", f"/api/v1/tyk-connections/{cid}")
        if isinstance(cc, dict) and cc.get("declared_mode") != "full":
            studio("PATCH", f"/api/v1/tyk-connections/{cid}", {"declared_mode": "full", "lock_version": cc.get("lock_version")})
            studio("POST", f"/api/v1/tyk-connections/{cid}/probe")
        for a in created["apps"]:
            print(" app", a, studio("DELETE", f"/api/v1/apps/{a}")[0])
        for s in created["servers"]:
            print(" server", s, studio("DELETE", f"/api/v1/mcp-servers/{s}?force=true")[0])
        for m in created["dash_mcps"]:
            print(" dash mcp", m, dash("DELETE", "/api/mcps/" + m)[0])
        for p in created["dash_policies"]:
            if p:
                print(" dash policy", p, dash("DELETE", "/api/portal/policies/" + p)[0])
        for t in created["targets"]:
            print(" target", t, studio("DELETE", f"/api/v1/webhooks/targets/{t}")[0])
        for a in created["dash_assets"]:
            print(" dash asset", a, dash("DELETE", "/api/assets/" + a)[0])
        for c in created["catalogues"]:
            print(" catalogue", c, studio("DELETE", f"/api/v1/tool-catalogues/{c}")[0])
        print(" connection", cid, studio("DELETE", f"/api/v1/tyk-connections/{cid}?force=true")[0])
    print("FAILURES:", len(failures), failures)
    sys.exit(1 if failures else 0)
