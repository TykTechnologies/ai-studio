package requestformat

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFor(t *testing.T) {
	for _, tc := range []struct {
		vendor, method, path, endpoint, want string
	}{
		{"openai", "POST", "/llm/rest/gpt/v1/chat/completions", "https://api.openai.com/v1", "openai"},
		{"openai", "POST", "/llm/rest/gpt/v1/responses", "https://api.openai.com/v1", OpenAIResponses},
		{"openai", "POST", "/llm/stream/gpt/responses", "https://api.openai.com/v1", OpenAIResponses},
		{"openai", "POST", "/llm/call/gpt/v1/responses/compact", "", OpenAIResponses},
		{"openai", "POST", "/llm/rest/gpt/v1/responses", "https://gw.example/openai/v1", OpenAIResponses},
		{"openai", "GET", "/llm/rest/gpt/v1/responses", "https://api.openai.com/v1", "openai"},
		{"openai", "POST", "/llm/rest/gpt/v1/embeddings", "https://api.openai.com/v1", OpenAIInput},
		{"openai", "POST", "/llm/rest/gpt/v1/decisions", "https://api.openai.com/v1", OpenAIDecisions},
		{"systemone", "POST", "/llm/rest/jev/v1/systemone", "", SystemOne},
		{"anthropic", "POST", "/llm/rest/claude/v1/messages", "https://api.anthropic.com", "anthropic"},
		// Chain formats are per vendor: an Anthropic LLM's /v1/responses is
		// whatever Anthropic makes of it.
		{"anthropic", "POST", "/llm/rest/claude/v1/responses", "", "anthropic"},
		{"bedrock", "POST", "/llm/rest/br/model/x/converse", "", "bedrock"},
		// Not the native pass-through: the vendor's format.
		{"openai", "POST", "/ai/gpt/v1/responses", "", "openai"},
		{"openai", "POST", "/llm/other/gpt/v1/responses", "", "openai"},
		// An endpoint that does not parse cannot name an operation.
		{"openai", "POST", "/llm/rest/gpt/v1/responses", "http://[::1", "openai"},
	} {
		if got := For(tc.vendor, tc.method, tc.path, tc.endpoint); got != tc.want {
			t.Errorf("For(%q, %q, %q, %q) = %q, want %q", tc.vendor, tc.method, tc.path, tc.endpoint, got, tc.want)
		}
	}
}

func TestCanonicalVendorPath(t *testing.T) {
	for _, tc := range []struct{ endpoint, rest, want string }{
		{"", "/v1/messages", "/v1/messages"},
		{"/v1", "/messages", "/v1/messages"},
		{"/v1", "/v1/messages", "/v1/messages"},
		{"/anthropic", "/v1/messages", "/v1/messages"},
		{"/anthropic/v1", "/messages", "/v1/messages"},
		{"/anthropic/v1", "/v1/messages", "/v1/messages"},
		{"/anthropic", "/anthropic", "/"},
	} {
		if got := CanonicalVendorPath(tc.endpoint, tc.rest); got != tc.want {
			t.Errorf("CanonicalVendorPath(%q, %q) = %q, want %q", tc.endpoint, tc.rest, got, tc.want)
		}
	}
}

// Plugins import this package, so it must not pull in the proxy, the models
// or GORM.
func TestNoHeavyDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if !strings.HasPrefix(dep, "github.com/") && !strings.Contains(dep, ".") {
			continue // standard library
		}
		switch {
		case dep == "github.com/TykTechnologies/midsommar/v2/pkg/requestformat",
			dep == "github.com/TykTechnologies/midsommar/v2/pkg/decisions":
		default:
			t.Errorf("requestformat depends on %s", dep)
		}
	}
}
