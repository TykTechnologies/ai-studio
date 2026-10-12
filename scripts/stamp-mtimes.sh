#!/usr/bin/env bash
# Gives every file in the checkout an mtime derived from its content, so
# Go's test cache survives a fresh CI checkout.
#
# `go test` caches a package's result only if the files its tests opened
# are unchanged, and it judges "unchanged" by size and mtime (files newer
# than a few seconds are never cached). A fresh checkout stamps every file
# with the clone time, so without this every test re-runs on every CI run.
#
# Each tracked file gets 2000-01-01 plus an offset taken from its git blob
# hash: the same content always gets the same mtime, and changed content
# gets a different one, so a cached result is reused only when what the
# test read is identical. Directories get a fixed mtime; their listings are
# hashed by name, not by mtime. Initialised submodules (enterprise/) are
# stamped too.
#
#   scripts/stamp-mtimes.sh [extra-file ...]
#
# Extra files are untracked files the tests read, such as the generated
# .env and the embed placeholders; they are stamped from their content in
# the same way. Run it after every step that writes into the tree (the
# placeholders, `make ent-link`), since writing a file changes the mtime
# of its directory.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

repos=(.)
while read -r _ path _; do
	[ -e "$path/.git" ] && repos+=("$path")
done < <(git submodule status 2>/dev/null | sed 's/^[-+U ]//')

python3 - "${repos[@]}" -- "$@" <<'EOF'
import hashlib
import os
import subprocess
import sys

BASE = 946684800  # 2000-01-01T00:00:00Z

args = sys.argv[1:]
sep = args.index("--")
repos, extras = args[:sep], args[sep + 1:]

dirs = {"."}
stamped = 0


def stamp(path, t):
    global stamped
    try:
        os.utime(path, (t, t), follow_symlinks=False)
        stamped += 1
    except FileNotFoundError:
        return
    parent = os.path.dirname(path)
    while parent and parent not in dirs:
        dirs.add(parent)
        parent = os.path.dirname(parent)


for repo in repos:
    out = subprocess.run(
        ["git", "-C", repo, "ls-files", "-s", "-z"],
        check=True, capture_output=True,
    ).stdout
    for entry in out.split(b"\0"):
        if not entry:
            continue
        meta, path = entry.split(b"\t", 1)
        mode, sha, _stage = meta.split()
        if mode == b"160000":  # submodule: stamped as a repo of its own
            continue
        path = os.path.normpath(os.path.join(repo, os.fsdecode(path)))
        stamp(path, BASE + int(sha[:7], 16))
    dirs.add(os.path.normpath(repo))

for path in extras:
    if not os.path.isfile(path):
        continue
    with open(path, "rb") as f:
        data = f.read()
    sha = hashlib.sha1(b"blob %d\0" % len(data) + data).hexdigest()
    stamp(os.path.normpath(path), BASE + int(sha[:7], 16))

for d in dirs:
    try:
        os.utime(d, (BASE, BASE), follow_symlinks=False)
    except FileNotFoundError:
        pass

print(f"stamp-mtimes: {stamped} files, {len(dirs)} directories")
EOF
