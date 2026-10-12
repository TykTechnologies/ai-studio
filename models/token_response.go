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

// IPriceAdjustedResponse is a token response billed at other than the model's
// list price, such as a Message Batches result billed at half price. The
// gateway multiplies the record's cost by PriceMultiplier (1 means list price).
// ServiceTier names the vendor tier that set it ("batch", "priority"...); it
// is stored on the chat record and proxy log (ServiceTier), picks the LLM's
// price multiplier override (LLM.PriceMultipliers), and reaches the hub with
// edge usage.
type IPriceAdjustedResponse interface {
	ITokenResponse
	ServiceTier() string
	PriceMultiplier() float64
}
