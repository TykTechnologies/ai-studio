#!/bin/bash
# jyx.52 capture: Codex CLI (npx, pinned) through the recording proxy, with an
# isolated CODEX_HOME. Keys are sourced into this process only.
set -euo pipefail
S="${OUT:?set OUT to a scratch dir outside the repo}"
P=http://127.0.0.1:47911
CODEX_VERSION="${CODEX_VERSION:-0.160.1}"
MODEL="${MODEL:-gpt-5.5}"
set -a; . "${VENDORS_ENV:?set VENDORS_ENV to an env file with VT_ANTHROPIC_API_KEY and VT_OPENAI_API_KEY}"; set +a

W="$S/work-cx"
H="$S/cx-home"
mkdir -p "$W" "$H"
cd "$W"
printf 'secret word: marmalade\nsecond line: tangerine\n' > notes.txt
cat > "$H/config.toml" <<EOF
model = "$MODEL"
model_provider = "capture"

[model_providers.capture]
name = "capture proxy"
base_url = "$P/openai/v1"
env_key = "OPENAI_API_KEY"
wire_api = "responses"
EOF

cx() { # label, args...
  local label=$1; shift
  curl -s "$P/__label?l=$label" >/dev/null
  env -i PATH="$PATH" HOME="$HOME" TERM=dumb \
    CODEX_HOME="$H" OPENAI_API_KEY="$VT_OPENAI_API_KEY" \
    npx -y "@openai/codex@$CODEX_VERSION" exec --json --skip-git-repo-check -s read-only "$@"
}

sid() { python3 -I -c 'import json,sys
found=None
for l in sys.stdin:
    try: e=json.loads(l)
    except Exception: continue
    s=e.get("thread_id") or e.get("session_id") or (e.get("msg") or {}).get("session_id")
    if s and not found: found=s
print(found)'; }

A=$(cx cx-A1-fresh "Run cat notes.txt and tell me the secret word. One short sentence." | tee "$S/cx-A1.jsonl" | sid)
echo "A=$A"
cx cx-A2-resume resume "$A" "Now what is on the second line? One short sentence." | tee "$S/cx-A2.jsonl" | sid
cx cx-A3-last resume --last "Repeat both words, comma separated." | tee "$S/cx-A3.jsonl" | sid
(cx cx-B-parallel "Run cat notes.txt and tell me the second word." > "$S/cx-B.jsonl") &
(cx cx-C-parallel "Run wc -l notes.txt and say how many lines it has." > "$S/cx-C.jsonl") &
wait
sid < "$S/cx-B.jsonl"; sid < "$S/cx-C.jsonl"
