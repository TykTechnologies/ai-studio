package bedrockVendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportsOneHourCacheTTL(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		{"anthropic.claude-3-7-sonnet-20250219-v1:0", false},
		{"us.anthropic.claude-3-7-sonnet-20250219-v1:0", false},
		{"us.anthropic.claude-3-5-sonnet-20241022-v2:0", false},
		{"anthropic.claude-3", false},
		{"anthropic.claude-3.5", false},
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", true},
		{"global.anthropic.claude-opus-4-5-20251101-v1:0", true},
		{"arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/abc123", true},
		{"anthropic.claude-30-sonnet-v1:0", true},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.want, SupportsOneHourCacheTTL(tt.modelID))
		})
	}
}
