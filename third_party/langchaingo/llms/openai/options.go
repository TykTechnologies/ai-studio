package openai

import "github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"

// WithMaxCompletionTokens sets the max_completion_tokens field for token generation.
// This is the recommended way to limit tokens with OpenAI models.
//
// Usage:
//
//	llm.GenerateContent(ctx, messages,
//	    openai.WithMaxCompletionTokens(100),
//	)
//
// Note: While llms.WithMaxTokens() still works for backward compatibility,
// WithMaxCompletionTokens is preferred for clarity when using OpenAI.
func WithMaxCompletionTokens(maxTokens int) llms.CallOption {
	return func(opts *llms.CallOptions) {
		opts.MaxTokens = maxTokens
	}
}

// WithLegacyMaxTokensField forces the use of the max_tokens field instead of max_completion_tokens.
// This is useful when connecting to older OpenAI-compatible inference servers that only
// support the max_tokens field and don't recognize max_completion_tokens.
//
// Usage:
//
//	llm.GenerateContent(ctx, messages,
//	    llms.WithMaxTokens(100),
//	    openai.WithLegacyMaxTokensField(), // Forces use of max_tokens field
//	)
func WithLegacyMaxTokensField() llms.CallOption {
	return func(opts *llms.CallOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		opts.Metadata["openai:use_legacy_max_tokens"] = true
	}
}

// WithReasoningEffort sets reasoning_effort on the request, verbatim. The
// accepted values ("none", "minimal", "low", "medium", "high", "xhigh", ...)
// and the default differ per model, so the value is not checked here, and
// nothing is sent when the option is absent or effort is empty: the model's
// own default then applies.
func WithReasoningEffort(effort string) llms.CallOption {
	return func(opts *llms.CallOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		opts.Metadata["openai:reasoning_effort"] = effort
	}
}

// extraBodyKey is where WithExtraBody keeps its fields in the call metadata.
const extraBodyKey = "openai:extra_body"

// WithExtraBody adds top-level fields to the chat completion request body as
// they are (service_tier, user, store, parallel_tool_calls, logprobs,
// top_logprobs, prompt_cache_key, safety_identifier, ...). Tyk: not in
// upstream. The OpenAI-shaped bridges use it to forward request parameters
// this driver has no option for, which it used to drop. A field the driver
// sets itself (model, messages, stream, temperature, ...) is never
// overridden. Several calls merge; a nil or empty map adds nothing.
func WithExtraBody(fields map[string]any) llms.CallOption {
	return func(opts *llms.CallOptions) {
		if len(fields) == 0 {
			return
		}
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		merged, _ := opts.Metadata[extraBodyKey].(map[string]any)
		if merged == nil {
			merged = make(map[string]any, len(fields))
		}
		for k, v := range fields {
			merged[k] = v
		}
		opts.Metadata[extraBodyKey] = merged
	}
}
