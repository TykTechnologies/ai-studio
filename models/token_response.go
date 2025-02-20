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
