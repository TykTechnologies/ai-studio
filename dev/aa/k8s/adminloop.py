"""Admin API load during a failover run (dev/aa/k8s): what gwbench does not do.

For each Studio in STUDIOS, two threads with a user of their own (Studio keeps
one session per user, so sharing one would log the other out):
  write   create and delete a group, every WRITE_INTERVAL seconds
  login   log in, every LOGIN_INTERVAL seconds
Every request is one JSON line on stdout: time, site, kind, status, error code
and duration. A request that gets no answer has status 0.
"""
import http.cookiejar, json, os, sys, threading, time, urllib.error, urllib.request

ORIGIN = os.environ["ORIGIN"]
PASSWORD = os.environ["PROBE_PASSWORD"]
lock = threading.Lock()


class Client:
    def __init__(self, base):
        self.base = base
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def call(self, method, path, body=None, timeout=60):
        token = self.opener.open(urllib.request.Request(self.base + "/csrf-token", headers={"Origin": ORIGIN}), timeout=timeout).headers.get("X-CSRF-Token", "")
        req = urllib.request.Request(self.base + path, method=method, data=None if body is None else json.dumps(body).encode(),
                                     headers={"Origin": ORIGIN, "Referer": ORIGIN + "/", "X-CSRF-Token": token, "Content-Type": "application/json"})
        with self.opener.open(req, timeout=timeout) as res:
            return res.status, res.read()


def timed(site, kind, fn):
    start = time.time()
    status, code, out = 0, "", None
    try:
        status, raw = fn()
        out = raw
    except urllib.error.HTTPError as e:
        status = e.code
        try:
            b = json.loads(e.read() or b"{}")
            err = b.get("error", b.get("code", ""))
            code = err.get("code", "") if isinstance(err, dict) else str(err)
        except Exception:
            code = "unparsed body"
    except Exception as e:
        code = type(e).__name__ + ": " + str(e)[:100]
    with lock:
        print(json.dumps({"t": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(start)) + ".%03dZ" % int(start % 1 * 1000),
                          "site": site, "kind": kind, "status": status, "code": code[:120], "ms": int((time.time() - start) * 1000)}))
        sys.stdout.flush()
    return status, out


def login_body(user):
    return {"data": {"type": "user", "attributes": {"email": user, "password": PASSWORD}}}


def login_loop(site, base, interval):
    user = "aa-probe-login-%s@tyk.io" % site
    while True:
        tick = time.time()
        timed(site, "login", lambda: Client(base).call("POST", "/auth/login", login_body(user)))
        time.sleep(max(0, interval - (time.time() - tick)))


def write_loop(site, base, interval):
    user = "aa-probe-write-%s@tyk.io" % site
    client, logged_in, n = Client(base), False, 0
    while True:
        tick = time.time()
        if not logged_in:
            status, _ = timed(site, "write-login", lambda: client.call("POST", "/auth/login", login_body(user)))
            logged_in = status == 200
        if logged_in:
            n += 1
            name = "aa-load-%s-%d-%d" % (site, n, int(tick))
            body = {"data": {"type": "group", "attributes": {"name": name, "members": [], "catalogues": [], "data_catalogues": [], "tool_catalogues": []}}}
            status, raw = timed(site, "write-create", lambda: client.call("POST", "/api/v1/groups", body))
            if status == 401:
                client, logged_in = Client(base), False
            elif 200 <= status < 300:
                try:
                    gid = json.loads(raw)["data"]["id"]
                    status, _ = timed(site, "write-delete", lambda: client.call("DELETE", "/api/v1/groups/%s" % gid))
                    if status == 401:
                        client, logged_in = Client(base), False
                except Exception:
                    pass
        time.sleep(max(0, interval - (time.time() - tick)))


for pair in os.environ["STUDIOS"].split(","):
    site, base = pair.split("=", 1)
    threading.Thread(target=write_loop, args=(site, base, float(os.environ.get("WRITE_INTERVAL", "1"))), daemon=True).start()
    threading.Thread(target=login_loop, args=(site, base, float(os.environ.get("LOGIN_INTERVAL", "2"))), daemon=True).start()
while True:
    time.sleep(3600)
