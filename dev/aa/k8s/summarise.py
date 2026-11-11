"""One run's numbers in a form that can be pasted into a report (dev/aa/k8s).

    summarise.py results/<run> [fault time, ISO UTC, e.g. 2026-11-11T07:40:12Z]

For each gwbench half: validity, requests, errors by status, release lag, the
added p50/p90/p99 against the direct arm, the sampled edge's CPU and memory,
and, when a fault time is given, the failed gateway-arm requests in 1 s
buckets from 10 s before it (when the first and last failure were, and the
worst seconds). For the admin loop: per site and kind, counts by status,
p50/p99 latency, and the failure window relative to the fault.
"""
import collections, datetime, glob, json, os, sys

out = sys.argv[1]
fault = None
if len(sys.argv) > 2:
    fault = datetime.datetime.fromisoformat(sys.argv[2].replace("Z", "+00:00")).timestamp()


def pct(values, q):
    values = sorted(values)
    return values[min(len(values) - 1, int(q * len(values)))] if values else None


for half in ("short", "stream"):
    for d in sorted(glob.glob(os.path.join(out, half, "*aa-*"))):
        if "calibration" in d:
            continue
        s = json.load(open(os.path.join(d, "summary.json")))
        cell = s["cells"][0]
        arms = {a["arm"]: a for a in cell["arms"]}
        g = arms["gateway"]
        over = {o["metric"]: o for o in cell["overhead"]}["ttft"]
        res = cell.get("resources") or {}
        print(f"== {half}: {s['scenario']} [{s.get('label', '')}] {'VALID' if s['valid'] else 'INVALID'}")
        for c in s["checks"]:
            if not c["passed"]:
                print(f"   failed check: {c['name']}: {c['detail'][:110]}")
        print(f"   gateway arm: {g['requests']} requests, {g['achieved_rps']:.0f}/s, errors {g['errors']} {g.get('error_kinds') or ''}; release lag p99 "
              f"{[c['detail'].split(' (')[0] for c in s['checks'] if 'gateway' in c['name'] and 'schedule' in c['name']]}")
        print("   added ttft ms: " + ", ".join(f"{p} {over[p]['estimate']:.2f} [{over[p]['lo']:.1f}, {over[p]['hi']:.1f}]" for p in ("p50", "p90", "p99")))
        if res:
            print(f"   sampled edge: cpu avg {res.get('cpu_cores_avg') or 0:.2f} max {res.get('cpu_cores_max') or 0:.2f} cores, rss max {res.get('rss_max_mb') or 0:.0f} MB, goroutines max {res.get('goroutines_max')}")
        status = collections.Counter()
        buckets = collections.Counter()
        fails = []
        with open(os.path.join(d, "results.jsonl")) as f:
            for line in f:
                r = json.loads(line)
                if r["arm"] != "gateway":
                    continue
                ok = 200 <= r.get("status", 0) < 300 and not r.get("err")
                status[r.get("status", 0) if not r.get("err") or r.get("status") else "no answer"] += 1
                if not ok:
                    t = r["intended_unix_ns"] / 1e9
                    fails.append(t)
                    buckets[int(t)] += 1
        print(f"   by status: {dict(sorted(status.items(), key=lambda kv: str(kv[0])))}")
        if fails:
            ref = fault if fault else min(fails)
            print(f"   failures: {len(fails)} ({100 * len(fails) / max(1, g['requests']):.3f}%), first {min(fails) - ref:+.1f} s, last {max(fails) - ref:+.1f} s relative to {'the fault' if fault else 'the first failure'}; "
                  f"worst seconds {[(round(k - ref), v) for k, v in buckets.most_common(5)]}")
        # latency by 10 s window around the fault
        if fault:
            near = [w for w in cell["windows"] if True]
            start = datetime.datetime.fromisoformat(cell["started"].replace("Z", "+00:00")).timestamp()
            rows = []
            for w in near:
                t = start + w["start_s"] - fault
                if -30 <= t <= 120:
                    a = w["arms"].get("gateway", {})
                    rows.append(f"{t:+.0f}s p99={a.get('ttft_p99') or a.get('p99_ttft') or '?'} err={a.get('error_rate', 0):.3f}")
            print("   windows around the fault: " + "; ".join(rows[:16]))

path = os.path.join(out, "adminloop.jsonl")
if os.path.exists(path):
    groups = collections.defaultdict(list)
    for line in open(path):
        try:
            r = json.loads(line)
        except Exception:
            continue
        groups[(r["site"], r["kind"])].append(r)
    for (site, kind), rows in sorted(groups.items()):
        st = collections.Counter((r["status"], r["code"][:40]) for r in rows)
        ms = [r["ms"] for r in rows if 200 <= r["status"] < 300]
        bad = [r for r in rows if not 200 <= r["status"] < 300]
        line = f"== admin {site} {kind}: {len(rows)} requests, {dict(collections.Counter(r['status'] for r in rows))}, ok p50 {pct(ms, 0.5)} ms p99 {pct(ms, 0.99)} ms max {max(ms) if ms else None} ms"
        if bad:
            ts = [datetime.datetime.fromisoformat(r["t"].replace("Z", "+00:00")).timestamp() for r in bad]
            ref = fault if fault else min(ts)
            line += f"; failures first {min(ts) - ref:+.1f} s, last {max(ts) - ref:+.1f} s; kinds {dict(st.most_common(4))}"
        slow = max(rows, key=lambda r: r["ms"])
        line += f"; slowest {slow['ms']} ms (status {slow['status']})"
        print(line)
if os.path.exists(os.path.join(out, "analytics-check.json")):
    a = json.load(open(os.path.join(out, "analytics-check.json")))
    print(f"== analytics: answered {a['answered_by_gateway']}, rows gained {a['proxy_log_rows_gained']}, difference {a['difference']} ({a['verdict']})")
