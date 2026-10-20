#!/bin/bash
# jyx.52 capture: OpenCode through the recording proxy, isolated XDG dirs.
# Keys are sourced into this process only.
set -euo pipefail
S="${OUT:?set OUT to a scratch dir outside the repo}"
P=http://127.0.0.1:47911
set -a; . "${VENDORS_ENV:?set VENDORS_ENV to an env file with VT_ANTHROPIC_API_KEY and VT_OPENAI_API_KEY}"; set +a

W="$S/work-oc"
mkdir -p "$W" "$S/oc-xdg/config" "$S/oc-xdg/data" "$S/oc-xdg/cache" "$S/oc-xdg/state"
cd "$W"
printf 'secret word: marmalade\nsecond line: tangerine\n' > notes.txt
cat > opencode.json <<EOF
{
  "\$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "share": "disabled",
  "provider": {
    "anthropic": {"options": {"baseURL": "$P/anthropic/v1", "apiKey": "{env:ANTHROPIC_API_KEY}"}},
    "openai": {"options": {"baseURL": "$P/openai/v1", "apiKey": "{env:OPENAI_API_KEY}"}}
  }
}
EOF

oc() { # label, args...
  local label=$1; shift
  curl -s "$P/__label?l=$label" >/dev/null
  env -i PATH="$PATH" HOME="$HOME" TERM=dumb \
    XDG_CONFIG_HOME="$S/oc-xdg/config" XDG_DATA_HOME="$S/oc-xdg/data" \
    XDG_CACHE_HOME="$S/oc-xdg/cache" XDG_STATE_HOME="$S/oc-xdg/state" \
    ANTHROPIC_API_KEY="$VT_ANTHROPIC_API_KEY" OPENAI_API_KEY="$VT_OPENAI_API_KEY" \
    OPENCODE_DISABLE_AUTOUPDATE=1 \
    opencode run --pure --format json "$@"
}

sid() { python3 -I -c 'import json,sys
found=None
for l in sys.stdin:
    try: e=json.loads(l)
    except Exception: continue
    s=e.get("sessionID") or (e.get("part") or {}).get("sessionID")
    if s and not found: found=s
print(found)'; }

MODEL="${1:-anthropic/claude-haiku-5-5}"
TAG="${2:-ant}"
A=$(oc oc-$TAG-A1-fresh -m "$MODEL" "Use the read tool on notes.txt and tell me the secret word. One short sentence." | tee "$S/oc-$TAG-A1.jsonl" | sid)
echo "A=$A"
oc oc-$TAG-A2-session -m "$MODEL" --session "$A" "Now what is on the second line? One short sentence." | tee "$S/oc-$TAG-A2.jsonl" | sid
oc oc-$TAG-A3-continue -m "$MODEL" --continue "Repeat both words, comma separated." | tee "$S/oc-$TAG-A3.jsonl" | sid
(oc oc-$TAG-B-parallel -m "$MODEL" "Use the read tool on notes.txt and tell me the second word." > "$S/oc-$TAG-B.jsonl") &
(oc oc-$TAG-C-parallel -m "$MODEL" "Use the read tool on notes.txt and say how many lines it has." > "$S/oc-$TAG-C.jsonl") &
wait
sid < "$S/oc-$TAG-B.jsonl"; sid < "$S/oc-$TAG-C.jsonl"
