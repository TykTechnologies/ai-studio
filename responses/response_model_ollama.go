package responses

import "time"

type OllamaGenerateResponse struct {
	CreatedAt          time.Time     `json:"created_at"`
	Model              string        `json:"model"`
	Response           string        `json:"response"`
	Context            []int         `json:"context,omitempty"`
	TotalDuration      time.Duration `json:"total_duration,omitempty"`
	LoadDuration       time.Duration `json:"load_duration,omitempty"`
	PromptEvalCount    int           `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration time.Duration `json:"prompt_eval_duration,omitempty"`
	EvalCount          int           `json:"eval_count,omitempty"`
	EvalDuration       time.Duration `json:"eval_duration,omitempty"`
	Done               bool          `json:"done"`
}

func (o *OllamaGenerateResponse) GetPromptTokens() int {
	return o.PromptEvalCount
}

func (o *OllamaGenerateResponse) GetResponseTokens() int {
	return o.EvalCount
}

func (o *OllamaGenerateResponse) GetChoiceCount() int {
	return 0
}

func (o *OllamaGenerateResponse) GetToolCount() int {
	return 0
}

func (o *OllamaGenerateResponse) GetModel() string {
	return o.Model
}

func (o *OllamaGenerateResponse) GetCacheWritePromptTokens() int {
	return 0
}

func (o *OllamaGenerateResponse) GetCacheReadPromptTokens() int {
	return 0
}
