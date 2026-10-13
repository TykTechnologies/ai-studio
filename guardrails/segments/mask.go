package segments

import (
	"strings"
	"unicode/utf8"
)

// Errors name where in a body something is wrong, but inside tool data the
// keys are data (a tool's JSON output, a record keyed by e-mail address), and
// errors reach compliance events, client responses and logs. So a path in an
// error is masked: tokens are printed only while they are array indexes or
// field names of the request schemas, and the rest is cut off with "…".

// schemaFields are the key names an error may print.
var schemaFields = func() map[string]bool {
	m := map[string]bool{}
	for _, list := range [][]string{
		openAIRootFields, openAIMessageFields, openAIPartFields, openAIToolCallFields, openAIFunctionFields, ollamaGenerateFields,
		anthropicRootFields, anthropicMessageFields, anthropicBlockFields, anthropicSourceFields,
		googleRootFields, googleContentFields, googleResponseFields, googleCodeFields,
		bedrockRootFields, bedrockMessageFields, bedrockBlockFields, bedrockToolUseFields, bedrockToolResultFields, bedrockResultFields,
		{
			// Google part members, both spellings.
			"functionResponse", "function_response", "functionCall", "function_call",
			"codeExecutionResult", "code_execution_result", "executableCode", "executable_code",
			"inlineData", "inline_data", "fileData", "file_data",
			// Documented top-level request parameters.
			"model", "max_tokens", "max_completion_tokens", "temperature", "top_p", "top_k", "stream",
			"stream_options", "stop", "stop_sequences", "tools", "tool_choice", "metadata", "n", "seed",
			"response_format", "user", "frequency_penalty", "presence_penalty", "logprobs", "top_logprobs",
			"parallel_tool_calls", "thinking", "input", "instructions", "generationConfig", "generation_config",
			"safetySettings", "safety_settings", "toolConfig", "tool_config", "cachedContent", "cached_content",
			"service_tier", "store", "reasoning_effort", "options", "format", "keep_alive", "images",
		},
	} {
		for _, f := range list {
			m[f] = true
		}
	}
	return m
}()

const maxMaskedPath = 200

// maskPath renders a JSON Pointer for an error: array indexes and schema
// field names as they are, then "…" at the first token that is neither.
func maskPath(ptr string) string {
	if ptr == "" {
		return "/"
	}
	var b strings.Builder
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		if _, isIndex := arrayIndex(tok); !isIndex && !schemaFields[tok] {
			b.WriteString("/…")
			break
		}
		b.WriteString("/" + tok)
	}
	s := b.String()
	if utf8.RuneCountInString(s) > maxMaskedPath {
		s = string([]rune(s)[:maxMaskedPath]) + "…"
	}
	return s
}

// keyName quotes key for an error when it is a schema field, and is empty
// otherwise.
func keyName(key string) string {
	if !schemaFields[key] {
		return ""
	}
	return ` "` + key + `"`
}
