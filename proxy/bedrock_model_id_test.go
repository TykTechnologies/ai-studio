package proxy

import "testing"

func TestExtractBedrockModelIDFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"converse", "/model/anthropic.claude-3-sonnet/converse", "anthropic.claude-3-sonnet"},
		{"converse-stream", "/model/anthropic.claude-3-haiku/converse-stream", "anthropic.claude-3-haiku"},
		{"with prefix", "/api/v1/model/us.amazon.nova-pro/converse", "us.amazon.nova-pro"},
		{"no model segment", "/api/v1/chat/completions", ""},
		{"model at end", "/model/", ""},
		{"empty path", "", ""},
		{"model with cross-region ARN", "/model/arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-v2/converse", "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-v2"},
		{"escaped ARN", "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A%3Afoundation-model%2Fanthropic.claude-v2/converse", "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-v2"},
		{"escaped inference profile on the gateway path", "/llm/rest/my-llm/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fus.anthropic.claude-3/converse-stream", "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBedrockModelIDFromPath(tt.path)
			if got != tt.want {
				t.Errorf("ExtractBedrockModelIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
