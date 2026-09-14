import { PRIVACY_LEVELS, privacyLevelForScore } from '../../common/privacy/privacyLevels';
import { getConfig } from "../../../../config";

export const generateSlug = (name) => {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
};

export const generateEndpointUrl = (path, name) => {
  const config = getConfig();
  const currentHost = window.location.hostname;
  const baseUrl = config.proxyURL || `//${currentHost}:9090`;
  const slug = generateSlug(name);
  return `${baseUrl}${path}${slug}/`;
};

export const getBudgetLimitText = (appData) => {
  if (!appData.setBudget) return 'not set';
  return `$${appData.monthlyBudget} per month`;
};

export const getOwnerName = (ownerData) => {
  if (ownerData.ownerType === 'current') {
    return ownerData.name || 'Current user';
  }
  return ownerData.formData?.name || 'New user';
};

const PROVIDER_CONFIG = {
  openai: {
    defaultModel: 'gpt-4o',
    getEndpointPath: (endpoint, model) => `${endpoint}v1/chat/completions`,
    headers: () => [
      '-H "Content-Type: application/json"',
      '-H "Authorization: Bearer YOUR_SECRET"'
    ],
    formatBody: 'openai'
  },
  anthropic: {
    defaultModel: 'claude-3-5-sonnet-20240620',
    getEndpointPath: (endpoint) => `${endpoint}v1/messages`,
    headers: () => [
      '-H "Content-Type: application/json"',
      '-H "x-api-key: YOUR_SECRET"',
      '-H "anthropic-version: 2023-06-01"'
    ],
    formatBody: 'openai'
  },
  google_ai: {
    defaultModel: 'gemini-1.5-flash',
    getEndpointPath: (endpoint, model) => `${endpoint}v1/models/${model}:generateContent?key=YOUR_SECRET`,
    headers: () => [
      '-H "Content-Type: application/json"'
    ],
    formatBody: 'google'
  },
  google: {
    defaultModel: 'gemini-1.5-flash',
    getEndpointPath: (endpoint, model) => `${endpoint}v1/models/${model}:generateContent?key=YOUR_SECRET`,
    headers: () => [
      '-H "Content-Type: application/json"'
    ],
    formatBody: 'google'
  },
  vertex: {
    defaultModel: 'gemini-pro',
    getEndpointPath: (endpoint, model) => `${endpoint}v1/models/${model}:generateContent`,
    headers: () => [
      '-H "Content-Type: application/json"',
      '-H "Authorization: Bearer YOUR_SECRET"'
    ],
    formatBody: 'google'
  },
  ollama: {
    defaultModel: 'llama3',
    getEndpointPath: (endpoint) => `${endpoint}api/chat`,
    headers: () => [
      '-H "Content-Type: application/json"',
      '-H "Authorization: YOUR_SECRET"'
    ],
    formatBody: 'openai'
  },
  huggingface: {
    defaultModel: 'mistralai/Mistral-7B-Instruct-v0.2',
    getEndpointPath: (endpoint, model) => `${endpoint}models/${model}`,
    headers: () => [
      '-H "Content-Type: application/json"',
      '-H "Authorization: Bearer YOUR_SECRET"'
    ],
    formatBody: 'openai'
  }
};

const formatBody = {
  openai: (model, promptText, temperature, maxTokens) => `{
    "model": "${model}",
    "messages": [
      {"role": "user", "content": "${promptText}"}
    ],
    "temperature": ${temperature},
    "max_tokens": ${maxTokens}
  }`,
  
  google: (model, promptText, temperature, maxTokens) => `{
    "contents": [{
      "parts":[{"text": "${promptText}"}]
    }],
    "model": "${model}",
    "generationConfig": {
      "temperature": ${temperature},
      "maxOutputTokens": ${maxTokens}
    }
  }`
};

// `secret` is optional: when given, the real app secret replaces the
// YOUR_SECRET placeholder so the example can be run as-is.
export const getCurlExample = (llmProvider = 'openai', llmName = 'OpenAI', secret = '') => {
  const promptText = "Generate a template OpenAPI Specification (OAS) for a simple TODO API.";
  const temperature = 0.7;
  const maxTokens = 1000;
  
  const config = PROVIDER_CONFIG[llmProvider] || PROVIDER_CONFIG.openai;
  const defaultModel = config.defaultModel;
  
  const formatter = formatBody[config.formatBody];
  const requestBody = formatter(defaultModel, promptText, temperature, maxTokens);
  
  const endpoint = generateEndpointUrl('/llm/rest/', llmName);
  const fullEndpointPath = config.getEndpointPath(endpoint, defaultModel);
  const headers = config.headers();
  
  const command = `curl -X POST "${fullEndpointPath}" \\
  ${headers.join(' \\\n  ')} \\
  -d '${requestBody}'`;

  return secret ? command.split('YOUR_SECRET').join(secret) : command;
};

export const validateEmail = (email) => {
  if (!email || !/\S+@\S+\.\S+/.test(email)) {
    return "Email is invalid";
  }
  return null;
};

export const validatePassword = (passwordCriteria) => {
  switch (false) {
    case passwordCriteria.length:
      return "Password must be at least 8 characters";
    case passwordCriteria.number:
      return "Password must contain a number";
    case passwordCriteria.special:
      return "Password must contain a special character";
    case passwordCriteria.uppercase:
      return "Password must contain an uppercase letter";
    case passwordCriteria.lowercase:
      return "Password must contain a lowercase letter";
    default:
      return null;
  }
};

// The quick-start keeps its named options, but the labels, descriptions and
// default scores come from the one privacy scale in
// components/common/privacy/privacyLevels.js so it never drifts from the
// LLM, tool and data source forms (UX review M4).
export const PRIVACY_LEVEL_SCORES = Object.fromEntries(
  PRIVACY_LEVELS.map((level) => [level.key, level.defaultScore])
);

export const PRIVACY_LEVEL_OPTIONS = PRIVACY_LEVELS.map((level) => ({
  value: level.key,
  label: level.label,
  description: level.description
}));

/** The named level a stored privacy_score falls in ("public" when unset). */
export const privacyLevelKeyForScore = (score) => privacyLevelForScore(score)?.key || 'public';

export const PRIVACY_BADGE_CONFIGS = {
  public: {
    icon: 'unlock',
    text: 'Public',
    textColor: 'text.successDefault',
    bgColor: 'border.successDefaultSubdued'
  },
  internal: {
    icon: 'lock',
    text: 'Internal',
    textColor: 'text.warningDefault',
    bgColor: 'border.warningDefaultSubdued'
  },
  confidential: {
    icon: 'lock-keyhole',
    text: 'Confidential',
    textColor: 'border.criticalHover',
    bgColor: 'border.criticalDefaultSubdue'
  },
  restricted: {
    icon: 'shield-keyhole',
    text: 'Restricted',
    textColor: 'background.surfaceCriticalDefault',
    bgColor: 'background.buttonPrimaryDefault'
  }
};