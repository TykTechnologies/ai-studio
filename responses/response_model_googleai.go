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
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Model string `json:"model"`
}

func (o *GoogleAIChatResponse) GetPromptTokens() int {
	return o.UsageMetadata.PromptTokenCount
}

func (o *GoogleAIChatResponse) GetResponseTokens() int {
	return o.UsageMetadata.CandidatesTokenCount
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
