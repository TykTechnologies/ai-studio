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

func TestHasTraversalSegment(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/v1/messages", false},
		{"/v1/models/gemini-2.5:generateContent", false},
		{"/v1/..messages", false}, // ".." inside a segment is not a traversal
		{"/v1/../../admin", true},
		{"/..", true},
		{"../v1/messages", true},
		{"/v1/messages/..", true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := hasTraversalSegment(tc.path); got != tc.want {
				t.Fatalf("hasTraversalSegment(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// canonicalVendorPath drops the endpoint's corporate prefix and keeps the
// vendor's version segment, so every row of the joinUpstreamPath matrix that
// sends .../v1/messages upstream canonicalises to /v1/messages.
func TestCanonicalVendorPath(t *testing.T) {
	tests := []struct {
		name, upstream, remaining, want string
	}{
		{"bare host", "", "/v1/messages", "/v1/messages"},
		{"root slash", "/", "/v1/messages", "/v1/messages"},
		{"versioned root", "/v1", "/v1/messages", "/v1/messages"},
		{"versioned root trailing slash", "/v1/", "/v1/messages", "/v1/messages"},
		{"proxy without version", "/anthropic", "/v1/messages", "/v1/messages"},
		{"proxy with version", "/anthropic/v1", "/v1/messages", "/v1/messages"},
		{"proxy with version, trailing slash", "/anthropic/v1/", "/v1/messages", "/v1/messages"},
		{"deep proxy prefix", "/acct/gw/anthropic/v1", "/v1/messages", "/v1/messages"},
		{"whole endpoint path repeated by caller", "/anthropic/v1", "/anthropic/v1/messages", "/v1/messages"},

		{"openai versioned root", "/v1", "/v1/chat/completions", "/v1/chat/completions"},
		{"openai proxy with version", "/openai/v1", "/v1/chat/completions", "/v1/chat/completions"},
		{"fireworks", "/inference/v1", "/v1/chat/completions", "/v1/chat/completions"},

		{"google v1beta under v1 endpoint", "/v1", "/v1beta/models/g:generateContent", "/v1/v1beta/models/g:generateContent"},
		{"google bare host", "/", "/v1beta/models/g:generateContent", "/v1beta/models/g:generateContent"},
		{"google proxy with v1beta", "/gemini/v1beta", "/v1beta/models/g:generateContent", "/v1beta/models/g:generateContent"},

		{"legacy shim path onto versioned endpoint", "/v1", "/messages", "/v1/messages"},
		{"legacy shim path onto bare host", "", "/messages", "/messages"},
		{"legacy shim path onto proxy with version", "/anthropic/v1", "/messages", "/v1/messages"},

		// Native resource paths keep their whole tail.
		{"batch results via proxy", "/anthropic/v1", "/v1/messages/batches/msgbatch_01.t1kab.cd/results", "/v1/messages/batches/msgbatch_01.t1kab.cd/results"},
		{"batch list, caller trailing slash", "/anthropic/v1/", "/v1/messages/batches/", "/v1/messages/batches"},

		// A prefix match is on whole segments: /anthropic does not strip /anthropicx.
		{"prefix is whole segments", "/anthropic", "/anthropicx/v1/messages", "/anthropicx/v1/messages"},
		// A segment that only looks like a version is part of the prefix.
		{"non-version last segment", "/api/vendor", "/v1/messages", "/v1/messages"},
		{"version-like but not", "/proxy/vx", "/v1/messages", "/v1/messages"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalVendorPath(tc.upstream, tc.remaining); got != tc.want {
				t.Fatalf("canonicalVendorPath(%q, %q) = %q, want %q", tc.upstream, tc.remaining, got, tc.want)
			}
		})
	}
}
