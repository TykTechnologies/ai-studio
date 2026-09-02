package metrics

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestGenAIProviderName(t *testing.T) {
	cases := map[string]string{
		"openai":      "openai",
		"anthropic":   "anthropic",
		"bedrock":     "aws.bedrock",
		"vertex":      "gcp.vertex_ai",
		"google_ai":   "gcp.gemini",
		"huggingface": "huggingface",
		"ollama":      "ollama",
	}
	for vendor, want := range cases {
		if got := GenAIProviderName(vendor); got != want {
			t.Errorf("GenAIProviderName(%q) = %q, want %q", vendor, got, want)
		}
	}
}

func TestGenAITokenTypeMapping(t *testing.T) {
	cases := map[string]string{
		"prompt":      "input",
		"completion":  "output",
		"cache_read":  "cache_read",
		"cache_write": "cache_write",
	}
	for in, want := range cases {
		if got := genaiTokenTypeOf(in); got != want {
			t.Errorf("genaiTokenTypeOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorTypeOf(t *testing.T) {
	if got := errorTypeOf(200); got != "" {
		t.Errorf("errorTypeOf(200) = %q, want empty", got)
	}
	if got := errorTypeOf(399); got != "" {
		t.Errorf("errorTypeOf(399) = %q, want empty", got)
	}
	if got := errorTypeOf(429); got != "429" {
		t.Errorf("errorTypeOf(429) = %q, want \"429\"", got)
	}
	if got := errorTypeOf(503); got != "503" {
		t.Errorf("errorTypeOf(503) = %q, want \"503\"", got)
	}
}

func TestGenAIMetricsExported(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordTokens(ctx, "bedrock", "claude-sonnet-4", "prompt", 120)
	RecordTokens(ctx, "bedrock", "claude-sonnet-4", "completion", 40)
	ObserveRequestDuration(ctx, "bedrock", "claude-sonnet-4", true, 2.5, http.StatusOK)
	ObserveTimeToFirstToken(ctx, "bedrock", "claude-sonnet-4", 0.4)
	ObserveTimePerOutputToken(ctx, "bedrock", "claude-sonnet-4", 2.0, 40)
	ObserveToolDuration(ctx, "search", 0.25)

	body := scrape(t, h)

	for _, want := range []string{
		"gen_ai_client_token_usage",
		"gen_ai_server_request_duration_seconds",
		"gen_ai_server_time_to_first_token_seconds",
		"gen_ai_server_time_per_output_token_seconds",
		"gen_ai_client_operation_duration_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in exported metrics", want)
		}
	}

	// Vendor and token categories must be exported under convention names/values.
	for _, want := range []string{
		`gen_ai_provider_name="aws.bedrock"`,
		`gen_ai_request_model="claude-sonnet-4"`,
		`gen_ai_token_type="input"`,
		`gen_ai_token_type="output"`,
		`gen_ai_operation_name="chat"`,
		`gen_ai_operation_name="execute_tool"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing attribute %s in exported metrics", want)
		}
	}
}

func TestGenAIErrorTypeOnlyOnFailure(t *testing.T) {
	h := Init()
	ctx := context.Background()

	ObserveRequestDuration(ctx, "openai", "gpt-4o", false, 0.2, http.StatusOK)
	ObserveRequestDuration(ctx, "openai", "gpt-4o-error", false, 0.2, http.StatusTooManyRequests)

	body := scrape(t, h)

	if !strings.Contains(body, `error_type="429"`) {
		t.Error("expected error_type=429 on the failing observation")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, `gen_ai_request_model="gpt-4o"`) && strings.Contains(line, "error_type=") {
			t.Errorf("successful request must not carry error_type: %s", line)
		}
	}
}

func TestTimePerOutputTokenDropsZeroTokenStreams(t *testing.T) {
	Init()
	ctx := context.Background()

	// A stream that produced no output tokens must not be recorded as a zero
	// observation, which would drag the latency distribution down.
	ObserveTimePerOutputToken(ctx, "openai", "gpt-4o", 5.0, 0)
	ObserveTimePerOutputToken(ctx, "openai", "gpt-4o", 0, 10)
}

func TestLegacyNamesCanBeDisabled(t *testing.T) {
	t.Setenv(EnvLegacyNames, "false")
	h := Init()
	ctx := context.Background()

	RecordTokens(ctx, "openai", "gpt-4o", "prompt", 10)
	ObserveRequestDuration(ctx, "openai", "gpt-4o", false, 1.0, http.StatusOK)
	ObserveToolDuration(ctx, "search", 0.1)

	body := scrape(t, h)

	for _, legacy := range []string{
		"aistudio_llm_tokens_total",
		"aistudio_llm_request_duration_seconds",
		"aistudio_tool_execution_duration_seconds",
	} {
		if strings.Contains(body, legacy) {
			t.Errorf("%s should not be exported when %s=false", legacy, EnvLegacyNames)
		}
	}
	if !strings.Contains(body, "gen_ai_client_token_usage") {
		t.Error("gen_ai instruments must still be exported when legacy names are off")
	}

	// Restore the default for any test that runs afterwards.
	os.Unsetenv(EnvLegacyNames)
	Init()
}

// TestGenAINoOpBeforeInit mirrors TestNoOpBeforeInit for the instruments added
// with the GenAI conventions. These are called from the proxy request path, so a
// nil instrument before Init would panic on a live request rather than fail a
// test.
func TestGenAINoOpBeforeInit(t *testing.T) {
	ctx := context.Background()
	old := initialized.Load()
	initialized.Store(false)
	defer initialized.Store(old)

	ObserveTimeToFirstToken(ctx, "openai", "gpt-4o", 0.4)
	ObserveTimePerOutputToken(ctx, "openai", "gpt-4o", 2.0, 40)
	RecordTokens(ctx, "openai", "gpt-4o", "prompt", 10)
	ObserveRequestDuration(ctx, "openai", "gpt-4o", false, 1.0, http.StatusOK)
	ObserveToolDuration(ctx, "search", 0.1)
}

// TestTokenTypesAreDistinguished guards the input/output mapping at the point it
// matters: a regression that collapsed both onto one label would make cost and
// throughput dashboards silently wrong rather than obviously broken.
func TestTokenTypesAreDistinguished(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordTokens(ctx, "openai", "gpt-4o", "prompt", 100)
	RecordTokens(ctx, "openai", "gpt-4o", "completion", 7)
	RecordTokens(ctx, "openai", "gpt-4o", "cache_read", 3)

	body := scrape(t, h)

	var inputSum, outputSum string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "gen_ai_client_token_usage_sum") {
			continue
		}
		if strings.Contains(line, `gen_ai_token_type="input"`) {
			inputSum = line
		}
		if strings.Contains(line, `gen_ai_token_type="output"`) {
			outputSum = line
		}
	}
	if inputSum == "" || outputSum == "" {
		t.Fatalf("input and output token series must both be present:\n%s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(inputSum), "100") {
		t.Errorf("input token sum is not 100: %s", inputSum)
	}
	if !strings.HasSuffix(strings.TrimSpace(outputSum), "7") {
		t.Errorf("output token sum is not 7: %s", outputSum)
	}
	// A category with no registered convention value must still be recorded
	// rather than dropped.
	if !strings.Contains(body, `gen_ai_token_type="cache_read"`) {
		t.Error("cache_read tokens were dropped instead of passed through")
	}
}

// TestZeroTokenRecordIsDropped keeps a zero from being recorded as a real
// observation, which would skew the distribution.
func TestZeroTokenRecordIsDropped(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordTokens(ctx, "openai", "gpt-4o", "cache_write", 0)

	body := scrape(t, h)
	if strings.Contains(body, `gen_ai_token_type="cache_write"`) {
		t.Error("a zero-token record should not produce a series")
	}
}
