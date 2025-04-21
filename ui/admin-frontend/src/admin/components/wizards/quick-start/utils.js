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

export const getCurlExample = (llmProvider = 'openai-gpt-4o') => {
  const baseUrl = getBaseUrl();
  const slug = generateSlug(llmProvider);
  return `curl -X POST "${baseUrl}/ai/${slug}/v1" \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "prompt": "Generate a template OpenAPI Specification (OAS) for a simple TODO API.",
    "temperature": 0.7,
    "max_tokens": 200
}'`;
};