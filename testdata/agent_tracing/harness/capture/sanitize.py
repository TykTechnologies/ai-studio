"""Turn capture.jsonl into committed fixtures: one JSONL per harness run.

Usage: sanitize.py <capture.jsonl> <out_dir>
"""
import json
import os
import re
import sys

src, out_dir = sys.argv[1], sys.argv[2]

DROP_HEADERS = {'content-length', 'accept-encoding', 'connection'}
SECRET = re.compile(r'^(authorization|x-api-key|api-key|cookie|openai-organization|openai-project|chatgpt-account-id)$', re.I)
DEVICE = re.compile(r'(\\*"device_id\\*":\\*")[0-9a-f]{64}')
ACCOUNT = re.compile(r'(\\*"account_uuid\\*":\\*")[^"\\]*')
# Codex: a stable per-install id in X-Codex-Turn-Metadata and client_metadata.
INSTALL = re.compile(r'(\\*"(?:x-codex-)?installation[_-]id\\*":\\*")[0-9a-fA-F-]{36}')

groups = {
    'cc-': 'claude-code-2.1.304/anthropic-messages.jsonl',
    'oc-ant-': 'opencode-1.18.23/anthropic-messages.jsonl',
    'oc-oai-': 'opencode-1.18.23/openai-responses.jsonl',
    'cx-': 'codex-cli-0.160.1/openai-responses.jsonl',
}
files = {}


def scrub(s):
    s = DEVICE.sub(r"\1<device_id:sha256>", s)
    s = INSTALL.sub(r"\1<installation_id>", s)
    return ACCOUNT.sub(r"\1<account_uuid>", s)


for line in open(src):
    r = json.loads(line)
    if r['method'] != 'POST':
        continue
    target = next((v for k, v in groups.items() if r['label'].startswith(k)), None)
    if target is None:
        continue
    headers = {}
    for k, v in sorted(r['headers'].items()):
        if k.lower() in DROP_HEADERS:
            continue
        headers[k] = '<redacted>' if SECRET.match(k) else v
    rec = {
        'seq': r['seq'],
        'run': r['label'],
        'method': r['method'],
        'path': r['path'],
        'status': r.get('status'),
        'headers': headers,
        'body': r.get('body'),
    }
    files.setdefault(target, []).append(scrub(json.dumps(rec, sort_keys=True)))

for target, lines in files.items():
    path = os.path.join(out_dir, target)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, 'w') as f:
        f.write('\n'.join(lines) + '\n')
    print(path, len(lines))
