"""One site's view of the whole stack, once a second (dev/aa/k8s, fault 7).

Runs on each site's hub node during the partition test and writes one JSON
line per second to stdout, where the kubelet keeps it: kubectl may not reach
the pods while the zones are cut, so everything is read after the heal.

Each line holds what this side could see of BOTH sides' HTTP endpoints: every
Studio's /ready and /health and every edge's /ready. "error" in place of an
answer means this side could not reach it, which is the observation. The
PostgreSQL nodes are asked by the pod's second container (50-observers.yaml).
"""
import concurrent.futures, json, os, sys, time, urllib.request

SIDE = os.environ["SIDE"]
HTTP = {
    "studio-a/ready": "http://studio-a:8080/ready", "studio-b/ready": "http://studio-b:8080/ready",
    "studio-a/health": "http://studio-a:8080/health", "studio-b/health": "http://studio-b:8080/health",
    "edge-a/ready": "http://edge-a:8080/ready", "edge-b/ready": "http://edge-b:8080/ready",
}
def http(url):
    start = time.time()
    try:
        with urllib.request.urlopen(url, timeout=2) as res:
            body = res.read(2000).decode("utf8", "replace")
            try:
                body = json.loads(body)
                body = {k: body[k] for k in ("status", "database", "config_source", "control_connected", "control_endpoint") if k in body}
            except Exception:
                body = body[:80]
            return {"status": res.status, "ms": int((time.time() - start) * 1000), "body": body}
    except Exception as e:
        return {"error": type(e).__name__ + ": " + str(e)[:80], "ms": int((time.time() - start) * 1000)}


pool = concurrent.futures.ThreadPoolExecutor(max_workers=10)
while True:
    tick = time.time()
    jobs = {name: pool.submit(http, url) for name, url in HTTP.items()}
    line = {"t": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(tick)) + ".%03dZ" % int(tick % 1 * 1000), "side": SIDE}
    line.update({name: job.result() for name, job in jobs.items()})
    print(json.dumps(line))
    sys.stdout.flush()
    time.sleep(max(0, 1 - (time.time() - tick)))
