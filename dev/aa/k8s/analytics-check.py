"""Combined analytics completeness check for one run (dev/aa/k8s).

gwbench's own check is switched off here because two generators share the
App. This does the same comparison once for both: the requests the gateway
answered (the gateway arm's results with an HTTP status, warm-up included,
as gwbench counts them) against the rows Studio's proxy log gained during
the run. More rows than answered requests means duplicates; fewer means loss.

    analytics-check.py results/<run>
"""
import collections, glob, json, os, sys

out = sys.argv[1]
answered = collections.Counter()
unanswered = 0
for path in glob.glob(os.path.join(out, "*", "**", "results.jsonl"), recursive=True):
    with open(path) as f:
        for line in f:
            r = json.loads(line)
            if r.get("arm") != "gateway":
                continue
            if r.get("status", 0) > 0:
                answered[r["status"]] += 1
            else:
                unanswered += 1
before = int(open(os.path.join(out, "proxy-log-rows.before")).read().strip() or 0)
after = int(open(os.path.join(out, "proxy-log-rows.after")).read().strip() or 0)
expected, gained = sum(answered.values()), after - before
result = {
    "answered_by_gateway": expected, "answered_by_status": dict(sorted(answered.items())),
    "no_answer": unanswered, "proxy_log_rows_gained": gained, "difference": gained - expected,
    "verdict": "complete" if gained == expected else ("duplicates or other traffic" if gained > expected else "loss"),
}
json.dump(result, open(os.path.join(out, "analytics-check.json"), "w"), indent=2)
print(json.dumps(result, indent=2))
