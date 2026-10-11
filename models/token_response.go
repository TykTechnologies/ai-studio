package models

type ITokenResponse interface {
	GetPromptTokens() int
	GetResponseTokens() int
	GetChoiceCount() int
	GetToolCount() int
	GetModel() string
	GetCacheWritePromptTokens() int
	GetCacheReadPromptTokens() int
}

// IDecisionResponse is the token response of a decision-model call. Besides
// token usage it reports how many questions the request asked and how many
// the model refused, for analytics.
type IDecisionResponse interface {
	ITokenResponse
	GetDecisionQuestions() int
	GetDecisionRefusals() int
}
