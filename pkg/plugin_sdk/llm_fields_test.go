package plugin_sdk

import "testing"

// The helpers forward to pkg/requestformat, whose tests cover each vendor;
// this checks the wiring with the Bedrock case plugins most often get wrong.
func TestLLMFieldHelpers(t *testing.T) {
	path := "/llm/call/br/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fus.amazon.nova-pro-v1%3A0/converse-stream"
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"Hi"}]}]}`)

	if got, want := RequestModel("bedrock", path, body), "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.amazon.nova-pro-v1:0"; got != want {
		t.Errorf("RequestModel = %q, want %q", got, want)
	}
	if !IsStreamingRequest("bedrock", path, body) {
		t.Error("IsStreamingRequest = false for converse-stream")
	}

	stream := []byte("data: {\"contentBlockIndex\":0,\"delta\":{\"text\":\"Hi\"}}\n\ndata: {\"usage\":{\"inputTokens\":12,\"outputTokens\":3,\"totalTokens\":15}}\n\n")
	want := Usage{InputTokens: 12, OutputTokens: 3, TotalTokens: 15, Reported: true}
	if got := ResponseUsage("bedrock", stream); got != want {
		t.Errorf("ResponseUsage = %+v, want %+v", got, want)
	}
}
