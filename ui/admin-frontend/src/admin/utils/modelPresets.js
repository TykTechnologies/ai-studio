const modelPresets = {
  // Default preset for custom configurations
  default: {
    model_name: "",
    temperature: 1.0,
    max_tokens: 4096,
    top_p: 1.0,
    top_k: 0, // 0 means disabled/not used
    min_length: 0,
    max_length: 0,
    repetition_penalty: 0, // 0 means disabled
  },

  // ========== OpenAI GPT-5 Family (December 2025) ==========
  // OpenAI recommends using temperature OR top_p, not both. We use temperature only.
  "OpenAI GPT-5.2": {
    model_name: "gpt-5.2",
    temperature: 1.0,
    max_tokens: 128000,
    top_p: 0, // Not sent - OpenAI recommends temperature OR top_p, not both
    top_k: 0, // Not supported by OpenAI
    min_length: 0,
    max_length: 400000,
    repetition_penalty: 0,
  },
  "OpenAI GPT-5.1": {
    model_name: "gpt-5.1",
    temperature: 1.0,
    max_tokens: 128000,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 400000,
    repetition_penalty: 0,
  },
  "OpenAI GPT-5": {
    model_name: "gpt-5",
    temperature: 1.0,
    max_tokens: 128000,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 400000,
    repetition_penalty: 0,
  },
  "OpenAI GPT-5 mini": {
    model_name: "gpt-5-mini",
    temperature: 1.0,
    max_tokens: 65536,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 400000,
    repetition_penalty: 0,
  },
  "OpenAI GPT-5 nano": {
    model_name: "gpt-5-nano",
    temperature: 1.0,
    max_tokens: 32768,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },

  // ========== OpenAI GPT-4 Family (Legacy) ==========
  "OpenAI GPT-4.1": {
    model_name: "gpt-4.1",
    temperature: 1.0,
    max_tokens: 32768,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 1047576,
    repetition_penalty: 0,
  },
  "OpenAI GPT-4o": {
    model_name: "gpt-4o",
    temperature: 1.0,
    max_tokens: 16384,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 0,
  },
  "OpenAI GPT-4o Mini": {
    model_name: "gpt-4o-mini",
    temperature: 1.0,
    max_tokens: 16384,
    top_p: 0,
    top_k: 0,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 0,
  },

  // ========== Anthropic Claude 4.5 Family (December 2025) ==========
  "Anthropic Claude Opus 4.5": {
    model_name: "claude-opus-4-5-20251101",
    temperature: 1.0,
    max_tokens: 64000,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },
  "Anthropic Claude Sonnet 4.5": {
    model_name: "claude-sonnet-4-5-20250929",
    temperature: 1.0,
    max_tokens: 64000,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000, // 1M available with beta header
    repetition_penalty: 0,
  },
  "Anthropic Claude Haiku 4.5": {
    model_name: "claude-haiku-4-5-20251001",
    temperature: 1.0,
    max_tokens: 64000,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },

  // ========== Anthropic Claude 4 Family ==========
  "Anthropic Claude Sonnet 4": {
    model_name: "claude-sonnet-4-20250514",
    temperature: 1.0,
    max_tokens: 16384,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },
  "Anthropic Claude Opus 4": {
    model_name: "claude-opus-4-20250514",
    temperature: 1.0,
    max_tokens: 32768,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },

  // ========== Anthropic Claude 3.5 Family (Legacy) ==========
  "Anthropic Claude 3.5 Sonnet": {
    model_name: "claude-3-5-sonnet-20241022",
    temperature: 1.0,
    max_tokens: 8192,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },
  "Anthropic Claude 3.5 Haiku": {
    model_name: "claude-3-5-haiku-20241022",
    temperature: 1.0,
    max_tokens: 8192,
    top_p: 1.0,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 0,
  },

  // ========== Google Gemini 3 Family (December 2025) ==========
  "Google Gemini 3 Pro": {
    model_name: "gemini-3-pro-preview",
    temperature: 1.0,
    max_tokens: 65536,
    top_p: 0.95,
    top_k: 40,
    min_length: 0,
    max_length: 1048576,
    repetition_penalty: 0,
  },

  // ========== Google Gemini 2.5 Family ==========
  "Google Gemini 2.5 Pro": {
    model_name: "gemini-2.5-pro",
    temperature: 1.0,
    max_tokens: 65536,
    top_p: 0.95,
    top_k: 40,
    min_length: 0,
    max_length: 1048576,
    repetition_penalty: 0,
  },
  "Google Gemini 2.5 Flash": {
    model_name: "gemini-2.5-flash",
    temperature: 1.0,
    max_tokens: 65536,
    top_p: 0.95,
    top_k: 40,
    min_length: 0,
    max_length: 1048576,
    repetition_penalty: 0,
  },
  "Google Gemini 2.5 Flash-Lite": {
    model_name: "gemini-2.5-flash-lite",
    temperature: 1.0,
    max_tokens: 65536,
    top_p: 0.95,
    top_k: 40,
    min_length: 0,
    max_length: 1048576,
    repetition_penalty: 0,
  },

  // ========== Google Gemini 2.0 Family (Legacy) ==========
  "Google Gemini 2.0 Flash": {
    model_name: "gemini-2.0-flash",
    temperature: 1.0,
    max_tokens: 8192,
    top_p: 0.95,
    top_k: 40,
    min_length: 0,
    max_length: 1048576,
    repetition_penalty: 0,
  },
  "Google Gemini 2.0 Flash-Lite": {
    model_name: "gemini-2.0-flash-lite",
    temperature: 1.0,
    max_tokens: 8192,
    top_p: 0.95,
    top_k: 40,
    min_length: 0,
    max_length: 1048576,
    repetition_penalty: 0,
  },

  // ========== Meta LLama Models ==========
  "Meta LLama 3.3": {
    model_name: "llama-3.3-70b",
    temperature: 1.0,
    max_tokens: 4096,
    top_p: 0.9,
    top_k: 40,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 0,
  },
  "Meta LLama 3.2": {
    model_name: "llama-3.2-90b",
    temperature: 1.0,
    max_tokens: 4096,
    top_p: 0.9,
    top_k: 40,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 0,
  },
};

export default modelPresets;
