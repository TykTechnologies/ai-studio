#!/usr/bin/env python3
"""Summarise make test-fips results (go test -json streams) as Markdown.

Usage: test-fips-summary.py <label>=<go-test-json-file> ...

For each module: whether its strict-mode canary (TestFIPSStrictCanary)
passed, the packages that failed and, per package, the failed tests or, for
a package that failed without a failing test (a build failure, an init
panic such as a non-approved algorithm at package init, a timeout), the end
of its output. Exits 1 if any module failed or its canary did not pass.
"""
import json
import sys
from collections import OrderedDict

CANARY = "TestFIPSStrictCanary"
TAIL = 12


def summarise(label, path):
    failed_tests = OrderedDict()  # package -> [test, ...] (top-level tests only)
    failed_pkgs = OrderedDict()   # package -> True
    pkg_output = {}               # package -> [line, ...]
    build_output = {}             # import path -> [line, ...]
    passed_pkgs = set()
    canary = None
    with open(path, encoding="utf-8", errors="replace") as f:
        for raw in f:
            raw = raw.strip()
            if not raw.startswith("{"):
                continue
            try:
                ev = json.loads(raw)
            except ValueError:
                continue
            action, pkg, test = ev.get("Action"), ev.get("Package", ""), ev.get("Test")
            if action == "build-output":
                # ImportPath may carry a test variant: "pkg [pkg.test]".
                key = ev.get("ImportPath", "").split(" ")[0]
                build_output.setdefault(key, []).append(ev.get("Output", ""))
                continue
            if test and test.split("/")[0] == CANARY and "/" not in test and action in ("pass", "fail", "skip"):
                canary = action
            if action == "output":
                # A panic in a goroutine a test started kills the binary
                # without a fail event for that test, so keep its lines too.
                pkg_output.setdefault(pkg, []).append(ev.get("Output", ""))
            elif action == "fail" and test:
                if "/" not in test:
                    failed_tests.setdefault(pkg, []).append(test)
            elif action == "fail":
                failed_pkgs[pkg] = True
            elif action == "pass" and not test:
                passed_pkgs.add(pkg)

    out = []
    ok = canary == "pass" and not failed_pkgs
    status = "OK" if ok else "FAILED"
    out.append(f"### {label}: {status}")
    out.append("")
    if canary == "pass":
        out.append(f"- Strict-mode canary ({CANARY}): passed (fips140 enabled and enforced, module v1.0.0)")
    else:
        out.append(f"- Strict-mode canary ({CANARY}): **{canary or 'did not run'}**; this run does not prove strict mode")
    out.append(f"- Packages passed: {len(passed_pkgs)}, failed: {len(failed_pkgs)}")
    if failed_pkgs:
        out.append("")
        out.append("| Package | Failed tests |")
        out.append("|---|---|")
        for pkg in failed_pkgs:
            tests = failed_tests.get(pkg)
            out.append(f"| `{pkg}` | {', '.join(f'`{t}`' for t in tests) if tests else '(no failing test: build failure, init panic or timeout; see below)'} |")
        for pkg in failed_pkgs:
            if failed_tests.get(pkg):
                continue
            lines = [l.rstrip("\n") for l in build_output.get(pkg, []) + pkg_output.get(pkg, []) if l.strip()]
            lines = [l for l in lines if not l.startswith(("FAIL\t", "FAIL ", "ok "))] or lines
            panics = [i for i, l in enumerate(lines) if l.startswith("panic: ")]
            if panics:
                shown, what = lines[panics[-1]:panics[-1] + TAIL], f"panic (first {TAIL} lines)"
            else:
                shown, what = lines[-TAIL:], f"output (last {TAIL} lines)"
            out.append("")
            out.append(f"<details><summary><code>{pkg}</code> {what}</summary>")
            out.append("")
            out.append("```")
            out.extend(shown)
            out.append("```")
            out.append("</details>")
    out.append("")
    return ok, "\n".join(out)


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    all_ok = True
    print("## FIPS strict-mode tests (GOFIPS140=v1.0.0, GODEBUG=fips140=only)")
    print()
    for arg in argv[1:]:
        label, _, path = arg.partition("=")
        ok, text = summarise(label, path)
        all_ok = all_ok and ok
        print(text)
    return 0 if all_ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
