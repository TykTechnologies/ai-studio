package responses

type GoogleAIChatResponse struct {
	Candidates []struct {
		Content struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string                 `json:"text"`
				Name string                 `json:"name"`
				Args map[string]interface{} `json:"args"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason  string `json:"finishReason"`
		SafetyRatings []struct {
			Category         string  `json:"category"`
			Probability      string  `json:"probability"`
			ProbabilityScore float64 `json:"probabilityScore"`
			Severity         string  `json:"severity"`
			SeverityScore    float64 `json:"severityScore"`
		} `json:"safetyRatings"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Model        string `json:"model"`
	ModelVersion string `json:"modelVersion"`
}

type GoogleAIStreamChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason  string `json:"finishReason"`
		Index         int    `json:"index"`
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

// type GoogleAIChatResponse struct {
// 	Candidates []struct {
// 		Content struct {
// 			Contents      []any  `json:"contents"`
// 			Tools         []any  `json:"tools"`
// 			CreateTime    string `json:"createTime"`
// 			UpdateTime    string `json:"updateTime"`
// 			UsageMetadata struct {
// 			} `json:"usageMetadata"`
// 			Name              string `json:"name"`
// 			DisplayName       string `json:"displayName"`
// 			Model             string `json:"model"`
// 			SystemInstruction struct {
// 			} `json:"systemInstruction"`
// 			ToolConfig struct {
// 			} `json:"toolConfig"`
// 		} `json:"content"`
// 		FinishReason     string `json:"finishReason"`
// 		SafetyRatings    []any  `json:"safetyRatings"`
// 		CitationMetadata struct {
// 		} `json:"citationMetadata"`
// 		TokenCount     int `json:"tokenCount"`
// 		AvgLogprobs    int `json:"avgLogprobs"`
// 		LogprobsResult struct {
// 		} `json:"logprobsResult"`
// 		Index int `json:"index"`
// 	} `json:"candidates"`
// 	PromptFeedback struct {
// 	} `json:"promptFeedback"`
// 	UsageMetadata struct {
// 		PromptTokenCount        int `json:"promptTokenCount"`
// 		CachedContentTokenCount int `json:"	ContentTokenCount"`
// 		CandidatesTokenCount    int `json:"candidatesTokenCount"`
// 		TotalTokenCount         int `json:"totalTokenCount"`
// 	} `json:"usageMetadata"`
// }

func (o *GoogleAIChatResponse) GetPromptTokens() int {
	return o.UsageMetadata.PromptTokenCount
}

func (o *GoogleAIChatResponse) GetResponseTokens() int {
	if o != nil {
		// INFO: We have to include thoughts tokens as they're priced like response tokens
		// More details: https://ai.google.dev/gemini-api/docs/thinking#go_5
		return o.UsageMetadata.CandidatesTokenCount +
			o.UsageMetadata.ThoughtsTokenCount
	}

	return 0
}

func (o *GoogleAIChatResponse) GetChoiceCount() int {
	return len(o.Candidates)
}

func (o *GoogleAIChatResponse) GetToolCount() int {
	cnt := 0
	for _, c := range o.Candidates {
		for _, p := range c.Content.Parts {
			if p.Name != "" && p.Args != nil {
				cnt++
			}
		}
	}

	return cnt
}

func (o *GoogleAIChatResponse) GetModel() string {
	return o.Model
}

func (o *GoogleAIChatResponse) SetModel(name string) {
	o.Model = name
}

func (o *GoogleAIChatResponse) GetCacheWritePromptTokens() int {
	return 0 // Google AI doesn't distinguish between write/read cache tokens
}

func (o *GoogleAIChatResponse) GetCacheReadPromptTokens() int {
	return o.UsageMetadata.CachedContentTokenCount // All cache tokens are considered read tokens
}
