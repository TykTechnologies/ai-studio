package traceschema

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The docs site publishes the schema; it must be the embedded one.
func TestPublishedCopyMatches(t *testing.T) {
	pub, err := os.ReadFile("../../../docs/site/public/schemas/agent-trace-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pub, V1) {
		t.Fatal("docs/site/public/schemas/agent-trace-v1.json differs from pkg/agenttrace/traceschema/agent-trace-v1.json; copy the schema over")
	}
}

const minimal = `{
  "media_type": "application/vnd.tyk.agent-trace+json; v=1",
  "version": 1,
  "scope": "session",
  "content_included": false,
  "session": {
    "id": "0192a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b",
    "app_id": 7,
    "source": "inferred",
    "status": "closed",
    "started_at": "2026-10-20T10:00:00Z",
    "last_activity_at": "2026-10-20T10:00:02.5Z",
    "totals": {"span_count": 1, "llm_call_count": 1, "tool_call_count": 0, "error_count": 0, "input_tokens": 10, "output_tokens": 5, "cost_micros": 42},
    "sampled": true,
    "content_mode": "off"
  },
  "agents": [],
  "traces": [{"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "turn_index": 0, "source": "tyk", "started_at": "2026-10-20T10:00:00Z", "status": "ok",
    "totals": {"span_count": 1, "llm_call_count": 1, "tool_call_count": 0, "error_count": 0, "input_tokens": 10, "output_tokens": 5, "cost_micros": 42}}],
  "spans": [{"span_id": "00f067aa0ba902b7", "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "kind": "llm_inference", "name": "chat gpt-4o",
    "start_time": "2026-10-20T10:00:00Z", "status": "ok", "inferred": false, "content_state": "not_captured"}]
}`

func TestValidateV1(t *testing.T) {
	if err := ValidateV1([]byte(minimal)); err != nil {
		t.Fatalf("minimal document: %v", err)
	}
	bad := []struct{ name, from, to, want string }{
		{"uppercase span id", `"00f067aa0ba902b7"`, `"00F067AA0BA902B7"`, "span_id"},
		{"short trace id", `"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "kind"`, `"trace_id": "4bf9", "kind"`, "trace_id"},
		{"non-UTC time", `"start_time": "2026-10-20T10:00:00Z"`, `"start_time": "2026-10-20T12:00:00+02:00"`, "start_time"},
		{"unknown kind", `"llm_inference"`, `"llm"`, "kind"},
		{"unknown field", `"inferred": false,`, `"inferred": false, "surprise": 1,`, "surprise"},
		{"wrong version", `"version": 1`, `"version": 2`, "version"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			doc := strings.Replace(minimal, tc.from, tc.to, 1)
			if doc == minimal {
				t.Fatalf("replacement %q not found", tc.from)
			}
			err := ValidateV1([]byte(doc))
			if err == nil {
				t.Fatal("expected a violation")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not mention %q: %v", tc.want, err)
			}
		})
	}
}
