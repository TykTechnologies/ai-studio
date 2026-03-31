package responses

// BedrockConverseResponse represents the JSON response from the Bedrock Converse API.
// Used by AnalyzeResponse to extract token usage for analytics.
type BedrockConverseResponse struct {
	Output struct {
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text,omitempty"`
			} `json:"content"`
		} `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
	Metrics struct {
		LatencyMs int `json:"latencyMs"`
	} `json:"metrics"`
}

func (r *BedrockConverseResponse) GetPromptTokens() int {
	return r.Usage.InputTokens
}

func (r *BedrockConverseResponse) GetResponseTokens() int {
	return r.Usage.OutputTokens
}

func (r *BedrockConverseResponse) GetChoiceCount() int {
	if len(r.Output.Message.Content) > 0 {
		return 1
	}
	return 0
}

func (r *BedrockConverseResponse) GetToolCount() int {
	return 0
}

func (r *BedrockConverseResponse) GetModel() string {
	return ""
}

func (r *BedrockConverseResponse) GetCacheWritePromptTokens() int {
	return 0
}

func (r *BedrockConverseResponse) GetCacheReadPromptTokens() int {
	return 0
}

// BedrockConverseStreamMetadataEvent represents the metadata event in a ConverseStream response.
// This is the final event containing token usage, sent as SSE by the SDK-mediated streaming proxy.
type BedrockConverseStreamMetadataEvent struct {
	Usage struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
	Metrics struct {
		LatencyMs int `json:"latencyMs"`
	} `json:"metrics"`
}

// BedrockConverseStreamContentBlockDelta represents a content block delta in a ConverseStream response.
type BedrockConverseStreamContentBlockDelta struct {
	ContentBlockIndex int `json:"contentBlockIndex"`
	Delta             struct {
		Text string `json:"text,omitempty"`
	} `json:"delta"`
}
