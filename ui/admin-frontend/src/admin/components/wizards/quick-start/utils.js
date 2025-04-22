import { getBaseUrl } from "../../../utils/urlUtils";

export const generateSlug = (name) => {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
};

export const generateEndpointUrl = (path, name) => {
  const baseUrl = getBaseUrl();
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

export const getCurlExample = (llmProvider = 'openai') => {
  const baseUrl = getBaseUrl();
  const promptText = "Generate a template OpenAPI Specification (OAS) for a simple TODO API.";
  const temperature = 0.7;
  const maxTokens = 1000;
  
  let defaultModel;
  let requestBody;
  
  const openaiFormat = (model) => `{
    "model": "${model}",
    "messages": [
      {"role": "user", "content": "${promptText}"}
    ],
    "temperature": ${temperature},
    "max_tokens": ${maxTokens}
}`;

  const googleFormat = (model) => `{
    "contents": [{
      "parts":[{"text": "${promptText}"}]
    }],
    "model": "${model}",
    "generationConfig": {
      "temperature": ${temperature},
      "maxOutputTokens": ${maxTokens}
    }
}`;
  
  switch (llmProvider) {
    case 'anthropic':
      defaultModel = 'claude-sonnet';
      requestBody = openaiFormat(defaultModel);
      break;
    case 'google':
    case 'google_ai':
      defaultModel = 'gemini-1.5-flash';
      requestBody = googleFormat(defaultModel);
      break;
    case 'vertex':
      defaultModel = 'gemini-pro';
      requestBody = googleFormat(defaultModel);
      break;
    case 'ollama':
      defaultModel = 'llama3';
      requestBody = openaiFormat(defaultModel);
      break;
    case 'huggingface':
      defaultModel = 'mistralai/Mistral-7B-Instruct-v0.2';
      requestBody = openaiFormat(defaultModel);
      break;
    default:
      defaultModel = 'gpt-4o';
      requestBody = openaiFormat(defaultModel);
      break;
  }
  
  return `curl -X POST "${baseUrl}/ai/${llmProvider}/v1" \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '${requestBody}'`;
};

export const validateEmail = (email) => {
  if (!email || !/\S+@\S+\.\S+/.test(email)) {
    return "Email is invalid";
  }
  return null;
};

export const validatePassword = (password, passwordCriteria) => {
  if (!passwordCriteria.length) {
    return "Password must be at least 8 characters";
  } else if (!passwordCriteria.number) {
    return "Password must contain a number";
  } else if (!passwordCriteria.special) {
    return "Password must contain a special character";
  } else if (!passwordCriteria.uppercase) {
    return "Password must contain an uppercase letter";
  }
  return null;
};