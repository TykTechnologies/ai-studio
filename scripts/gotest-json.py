#!/usr/bin/env python3
"""Test timings for CI: keep go test's -json stream, print the -v log.

    go test -json ... | scripts/gotest-json.py tee FILE.json
        Writes every line of the stream to FILE.json and prints the test
        output to stdout: the text `go test -v` prints, which the events
        carry verbatim, one block per package as it finishes. Lines that
        are not JSON are printed as they are. Use it with
        `set -o pipefail` so the step keeps go test's exit status.

    scripts/gotest-json.py summary [--packages N] [--tests N] LABEL=FILE.json ...
        Prints a Markdown summary of one or more streams: package and test
        counts (how many packages were "(cached)"), the slowest packages and
        the slowest top-level tests. CI appends it to $GITHUB_STEP_SUMMARY.

Standard library only: it runs on a bare runner before or after Go.
"""

import argparse
import json
import signal
import sys


def tee(path):
    out = sys.stdout.buffer
    # go test -v prints each package's output as one block; -json streams
    # the events of all running packages interleaved. Buffer per package
    # and print the block when the package finishes, so the log reads as
    # before (blocks come in completion order, not import-path order).
    pending = {}

    def flush(pkg):
        out.write(b"".join(pending.pop(pkg, [])))
        out.flush()

    def handle(raw):
        try:
            ev = json.loads(raw)
        except ValueError:
            ev = None
        if not (isinstance(ev, dict) and "Action" in ev):
            out.write(raw)
            out.flush()
            return
        action = ev["Action"]
        pkg = ev.get("Package")
        text = ev.get("Output", "").encode("utf-8", "replace")
        if action == "build-output" or (action == "output" and not pkg):
            # Compiler errors: -json reports them on stdout (Go 1.24+), -v
            # on stderr as soon as the build fails.
            out.write(text)
            out.flush()
        elif action == "output":
            pending.setdefault(pkg, []).append(text)
        elif pkg and not ev.get("Test") and action in ("pass", "fail", "skip"):
            flush(pkg)

    # On a cancelled job, keep reading until go test exits (SIGINT), or
    # print what is buffered before exiting (SIGTERM).
    signal.signal(signal.SIGINT, signal.SIG_IGN)
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    with open(path, "wb") as f:
        try:
            for raw in sys.stdin.buffer:
                f.write(raw)
                handle(raw)
        finally:
            # A stream cut short (timeout, cancel): print what there is.
            for pkg in list(pending):
                flush(pkg)
    return 0


def load(label, path):
    """Returns (packages, tests) for one stream. A top-level test's time is
    its own or its slowest subtest's, whichever is longer: go test reports
    0s for a test whose subtests all call t.Parallel."""
    pkgs = {}
    tests = {}
    try:
        f = open(path, "rb")
    except OSError as e:
        print(f"<!-- {label}: {e} -->", file=sys.stderr)
        return pkgs, []
    with f:
        for raw in f:
            try:
                ev = json.loads(raw)
            except ValueError:
                continue
            if not isinstance(ev, dict):
                continue
            pkg = ev.get("Package")
            if not pkg:
                continue
            action = ev.get("Action")
            test = ev.get("Test")
            p = pkgs.setdefault(pkg, {"label": label, "pkg": pkg, "result": None,
                                      "elapsed": 0.0, "cached": False, "notests": False})
            if test:
                if action in ("pass", "fail", "skip"):
                    top = test.split("/", 1)[0]
                    tc = tests.setdefault((pkg, top), {"label": label, "pkg": pkg, "test": top,
                                                       "result": None, "elapsed": 0.0})
                    tc["elapsed"] = max(tc["elapsed"], ev.get("Elapsed", 0.0) or 0.0)
                    if test == top:
                        tc["result"] = action
                continue
            if action == "output":
                text = ev.get("Output", "")
                # The package's result line: "ok  \tpkg\t(cached)" or
                # "?   \tpkg\t[no test files]".
                if text.startswith("ok") and "(cached)" in text:
                    p["cached"] = True
                if text.startswith("?") and "[no test files]" in text:
                    p["notests"] = True
            elif action in ("pass", "fail", "skip"):
                p["result"] = action
                p["elapsed"] = ev.get("Elapsed", 0.0) or 0.0
    # A cached package replays its old results; its tests did not run.
    ran = [t for t in tests.values() if t["result"] and not pkgs[t["pkg"]]["cached"]]
    return pkgs, ran


def short(pkg):
    for prefix in ("github.com/TykTechnologies/midsommar/v2/",
                   "github.com/TykTechnologies/midsommar/microgateway/",
                   "github.com/TykTechnologies/ai-studio-enterprise/v2/"):
        if pkg.startswith(prefix):
            return pkg[len(prefix):]
    return pkg


def fmt_secs(s):
    return f"{s:.1f}s" if s < 60 else f"{int(s // 60)}m{s % 60:04.1f}s"


def summary(args):
    allpkgs = []
    alltests = []
    lines = []
    lines.append("| Step | Packages with tests | Ran | (cached) | Failed | No test files | Sum of package times |")
    lines.append("|---|---:|---:|---:|---:|---:|---:|")
    for spec in args.files:
        label, _, path = spec.partition("=")
        if not path:
            label, path = spec, spec
        pkgs, tests = load(label, path)
        withtests = [p for p in pkgs.values() if p["result"] and not p["notests"]]
        cached = [p for p in withtests if p["cached"]]
        failed = [p for p in withtests if p["result"] == "fail"]
        notests = [p for p in pkgs.values() if p["notests"]]
        ran = [p for p in withtests if not p["cached"]]
        lines.append(f"| {label} | {len(withtests)} | {len(ran)} | {len(cached)} | {len(failed)} "
                     f"| {len(notests)} | {fmt_secs(sum(p['elapsed'] for p in ran))} |")
        allpkgs.extend(ran)
        alltests.extend(tests)

    lines.append("")
    lines.append(f"<details open><summary>Slowest {args.packages} packages (excluding cached)</summary>")
    lines.append("")
    lines.append("| Step | Package | Time | Result |")
    lines.append("|---|---|---:|---|")
    for p in sorted(allpkgs, key=lambda p: -p["elapsed"])[:args.packages]:
        lines.append(f"| {p['label']} | `{short(p['pkg'])}` | {fmt_secs(p['elapsed'])} | {p['result']} |")
    lines.append("")
    lines.append("</details>")
    lines.append("")
    lines.append(f"<details open><summary>Slowest {args.tests} top-level tests (parallel subtests: the slowest one)</summary>")
    lines.append("")
    lines.append("| Step | Package | Test | Time | Result |")
    lines.append("|---|---|---|---:|---|")
    for t in sorted(alltests, key=lambda t: -t["elapsed"])[:args.tests]:
        lines.append(f"| {t['label']} | `{short(t['pkg'])}` | `{t['test']}` | {fmt_secs(t['elapsed'])} | {t['result']} |")
    lines.append("")
    lines.append("</details>")
    print("\n".join(lines))
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    t = sub.add_parser("tee")
    t.add_argument("file")
    s = sub.add_parser("summary")
    s.add_argument("--packages", type=int, default=15)
    s.add_argument("--tests", type=int, default=25)
    s.add_argument("files", nargs="+", metavar="LABEL=FILE")
    args = ap.parse_args()
    if args.cmd == "tee":
        return tee(args.file)
    return summary(args)


if __name__ == "__main__":
    sys.exit(main())
