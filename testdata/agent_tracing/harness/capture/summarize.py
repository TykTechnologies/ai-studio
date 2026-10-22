import json
import sys

S = sys.argv[2] if len(sys.argv) > 2 else 'capture.jsonl'
prefix = sys.argv[1] if len(sys.argv) > 1 else ''
skip = {'content-length', 'accept-encoding', 'connection', 'accept', 'content-type', 'user-agent', 'x-api-key', 'authorization'}
for l in open(S):
    r = json.loads(l)
    if not r['label'].startswith(prefix):
        continue
    b = r.get('body') or {}
    h = {k: v for k, v in r['headers'].items() if k.lower() not in skip}
    print(r['seq'], r['label'], r['method'], r['path'], r.get('status'))
    print('   ua:', r['headers'].get('User-Agent'))
    print('   hdr:', json.dumps(h, sort_keys=True))
    if isinstance(b, dict):
        print('   body keys:', sorted(b.keys()))
        for k in ('metadata', 'user', 'prompt_cache_key', 'previous_response_id', 'conversation', 'store', 'safety_identifier', 'session_id'):
            if k in b:
                print('   %s: %s' % (k, json.dumps(b[k])))
        msgs = b.get('messages') or b.get('input')
        if isinstance(msgs, list):
            print('   msgs:', len(msgs), 'tools:', len(b.get('tools', [])), 'model:', b.get('model'))
