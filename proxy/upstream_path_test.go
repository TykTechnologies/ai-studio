package proxy

import "testing"

func TestJoinUpstreamPath(t *testing.T) {
	tests := []struct {
		name, upstream, remaining, want string
	}{
		// Anthropic: native callers and the bridge both send /v1/messages.
		{"bare host", "", "/v1/messages", "/v1/messages"},
		{"root slash", "/", "/v1/messages", "/v1/messages"},
		{"versioned root", "/v1", "/v1/messages", "/v1/messages"},
		{"versioned root trailing slash", "/v1/", "/v1/messages", "/v1/messages"},
		{"proxy without version", "/anthropic", "/v1/messages", "/anthropic/v1/messages"},
		{"proxy with version", "/anthropic/v1", "/v1/messages", "/anthropic/v1/messages"},
		{"proxy with version, trailing slash", "/anthropic/v1/", "/v1/messages", "/anthropic/v1/messages"},
		{"deep proxy prefix", "/acct/gw/anthropic/v1", "/v1/messages", "/acct/gw/anthropic/v1/messages"},
		{"whole endpoint path repeated by caller", "/anthropic/v1", "/anthropic/v1/messages", "/anthropic/v1/messages"},

		// OpenAI-compatible endpoints get the same treatment.
		{"openai versioned root", "/v1", "/v1/chat/completions", "/v1/chat/completions"},
		{"openai proxy with version", "/openai/v1", "/v1/chat/completions", "/openai/v1/chat/completions"},
		{"fireworks", "/inference/v1", "/v1/chat/completions", "/inference/v1/chat/completions"},

		// Overlap is whole segments only: /v1 must not swallow /v1beta.
		{"google v1beta under v1 endpoint", "/v1", "/v1beta/models/g:generateContent", "/v1/v1beta/models/g:generateContent"},
		{"google bare host", "/", "/v1beta/models/g:generateContent", "/v1beta/models/g:generateContent"},

		// A caller path with no version segment (legacy shim shape) still joins.
		{"legacy shim path onto versioned endpoint", "/v1", "/messages", "/v1/messages"},
		{"legacy shim path onto bare host", "", "/messages", "/messages"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinUpstreamPath(tc.upstream, tc.remaining); got != tc.want {
				t.Fatalf("joinUpstreamPath(%q, %q) = %q, want %q", tc.upstream, tc.remaining, got, tc.want)
			}
		})
	}
}
