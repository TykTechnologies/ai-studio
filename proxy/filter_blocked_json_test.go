package proxy

import (
	"encoding/json"
	"testing"
)

// Issue #402: streaming-filter block payloads must be built with
// encoding/json — fmt.Sprintf produced malformed JSON when interpolated
// values contained quotes or non-JSON text.

func TestBuildFilterBlockedErrorChunk(t *testing.T) {
	cases := []string{
		"simple reason",
		`reason with "quotes" and \ backslashes`,
		"newlines\nand\ttabs",
		`{"nested":"json"}`,
	}
	for _, msg := range cases {
		chunk := buildFilterBlockedErrorChunk(msg)

		var parsed map[string]string
		if err := json.Unmarshal(chunk, &parsed); err != nil {
			t.Fatalf("error chunk is not valid JSON for %q: %v (payload: %s)", msg, err, chunk)
		}
		if parsed["error"] == "" {
			t.Errorf("expected non-empty error field for %q", msg)
		}
	}
}

func TestBuildFilterBlockedAnalyticsBody(t *testing.T) {
	partials := []string{
		`data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n", // SSE — not JSON
		`text with "quotes"`,
		"",
	}
	for _, partial := range partials {
		body := buildFilterBlockedAnalyticsBody(`blocked "reason"`, 3, partial)

		var parsed struct {
			FilterBlocked   bool   `json:"filter_blocked"`
			BlockReason     string `json:"block_reason"`
			ChunkIndex      int    `json:"chunk_index"`
			PartialResponse string `json:"partial_response"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("analytics body is not valid JSON for partial %q: %v (payload: %s)", partial, err, body)
		}
		if !parsed.FilterBlocked {
			t.Error("filter_blocked should be true")
		}
		if parsed.BlockReason != `blocked "reason"` {
			t.Errorf("block_reason mangled: %q", parsed.BlockReason)
		}
		if parsed.ChunkIndex != 3 {
			t.Errorf("chunk_index mangled: %d", parsed.ChunkIndex)
		}
		if parsed.PartialResponse != partial {
			t.Errorf("partial_response mangled: %q != %q", parsed.PartialResponse, partial)
		}
	}
}
