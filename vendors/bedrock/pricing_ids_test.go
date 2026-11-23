package bedrockVendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPricingModelIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		want []string
	}{
		{"plain model id", "amazon.nova-lite-v1:0", nil},
		{"system profile id", "us.amazon.nova-lite-v1:0", []string{"amazon.nova-lite-v1:0"}},
		{"apac profile id", "apac.amazon.nova-lite-v1:0", []string{"amazon.nova-lite-v1:0"}},
		{"eu profile id", "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", []string{"anthropic.claude-sonnet-4-5-20250929-v1:0"}},
		{"global profile id", "global.anthropic.claude-sonnet-4-5-20250929-v1:0", []string{"anthropic.claude-sonnet-4-5-20250929-v1:0"}},
		{"us-gov profile id", "us-gov.anthropic.claude-3-5-sonnet-20240620-v1:0", []string{"anthropic.claude-3-5-sonnet-20240620-v1:0"}},
		{"profile ARN",
			"arn:aws:bedrock:ap-southeast-2:123456789012:inference-profile/apac.amazon.nova-lite-v1:0",
			[]string{"apac.amazon.nova-lite-v1:0", "amazon.nova-lite-v1:0"}},
		{"profile ARN, other partition, region and account",
			"arn:aws-us-gov:bedrock:us-gov-west-1:999999999999:inference-profile/us-gov.anthropic.claude-3-5-sonnet-20240620-v1:0",
			[]string{"us-gov.anthropic.claude-3-5-sonnet-20240620-v1:0", "anthropic.claude-3-5-sonnet-20240620-v1:0"}},
		{"profile ARN in aws-cn", "arn:aws-cn:bedrock:cn-north-1:1:inference-profile/us.amazon.nova-pro-v1:0",
			[]string{"us.amazon.nova-pro-v1:0", "amazon.nova-pro-v1:0"}},
		{"profile ARN with an upper-case prefix", "ARN:AWS:Bedrock:us-east-1:1:inference-profile/us.amazon.nova-lite-v1:0",
			[]string{"us.amazon.nova-lite-v1:0", "amazon.nova-lite-v1:0"}},
		{"foundation-model ARN", "arn:aws:bedrock:us-east-1::foundation-model/amazon.nova-lite-v1:0", []string{"amazon.nova-lite-v1:0"}},

		// Opaque ids name no model: never guessed.
		{"application inference profile", "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/abc123def456", nil},
		{"application inference profile named like a cheap model",
			"arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/us.amazon.nova-micro-v1:0", nil},
		{"provisioned model", "arn:aws:bedrock:us-east-1:123456789012:provisioned-model/abcd1234", nil},
		{"custom model", "arn:aws:bedrock:us-east-1:123456789012:custom-model/amazon.nova-lite-v1:0/abc", nil},
		{"prompt router", "arn:aws:bedrock:us-east-1:123456789012:default-prompt-router/anthropic.claude:1", nil},
		{"ARN of another service", "arn:aws:sagemaker:us-east-1:1:inference-profile/us.amazon.nova-lite-v1:0", nil},
		{"truncated ARN", "arn:aws:bedrock:us-east-1", nil},
		{"profile ARN with a nested path", "arn:aws:bedrock:us-east-1:1:inference-profile/us.amazon.nova-lite-v1:0/extra", nil},
		{"foundation-model ARN without a model id shape", "arn:aws:bedrock:us-east-1::foundation-model/cheap", nil},

		// A geography prefix is stripped from a profile id only.
		{"us. on a name that is no model id", "us.cheap", nil},
		{"us. alone", "us.", nil},
		{"prefix in the middle", "amazon.us.nova", nil},
		{"upper-case prefix is not a profile id", "US.amazon.nova-lite-v1:0", nil},
		{"unknown geography", "mars.amazon.nova-lite-v1:0", nil},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, PricingModelIDs(tc.id))
		})
	}
}
