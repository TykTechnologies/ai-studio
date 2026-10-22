#!/bin/bash
# jyx.52 capture: Claude Code through the recording proxy. Keys are sourced
# into this process only, never written anywhere.
set -euo pipefail
S="${OUT:?set OUT to a scratch dir outside the repo}"
P=http://127.0.0.1:47911
set -a; . "${VENDORS_ENV:?set VENDORS_ENV to an env file with VT_ANTHROPIC_API_KEY and VT_OPENAI_API_KEY}"; set +a

mkdir -p "$S/cc-config" "$S/work-cc"
cd "$S/work-cc"
printf 'secret word: marmalade\nsecond line: tangerine\n' > notes.txt

cc() { # label, args...
  local label=$1; shift
  curl -s "$P/__label?l=$label" >/dev/null
  env -i PATH="$PATH" HOME="$HOME" TERM=dumb \
    CLAUDE_CONFIG_DIR="$S/cc-config" \
    ANTHROPIC_API_KEY="$VT_ANTHROPIC_API_KEY" \
    ANTHROPIC_BASE_URL="$P/anthropic" \
    DISABLE_AUTOUPDATER=1 \
    claude -p --model haiku --allowedTools Read --output-format json "$@"
}

sid() { python3 -c 'import json,sys; print(json.load(sys.stdin)["session_id"])'; }

case "${1:-all}" in
all)
  A=$(cc cc-A1-fresh "Use the Read tool on notes.txt and tell me the secret word. One short sentence." | tee "$S/cc-A1.json" | sid)
  echo "A=$A"
  cc cc-A2-resume --resume "$A" "Now what is on the second line? One short sentence." | tee "$S/cc-A2.json" | sid
  cc cc-A3-continue --continue "Repeat both words, comma separated." | tee "$S/cc-A3.json" | sid
  # Two parallel fresh sessions.
  (cc cc-B-parallel "Use the Read tool on notes.txt and tell me the second word. One short sentence." > "$S/cc-B.json") &
  (cc cc-C-parallel "Use the Read tool on notes.txt and say how many lines it has. One short sentence." > "$S/cc-C.json") &
  wait
  sid < "$S/cc-B.json"; sid < "$S/cc-C.json"
  ;;
subagent)
  curl -s "$P/__label?l=cc-D-subagent" >/dev/null
  env -i PATH="$PATH" HOME="$HOME" TERM=dumb \
    CLAUDE_CONFIG_DIR="$S/cc-config" \
    ANTHROPIC_API_KEY="$VT_ANTHROPIC_API_KEY" \
    ANTHROPIC_BASE_URL="$P/anthropic" \
    DISABLE_AUTOUPDATER=1 \
    claude -p --model haiku --allowedTools "Agent,Task,Read" --output-format json \
    "Delegate this to a sub-agent using the Agent (Task) tool: the sub-agent must read notes.txt and report the secret word. Then tell me the word in one sentence." \
    | tee "$S/cc-D.json" | sid
  ;;
esac
