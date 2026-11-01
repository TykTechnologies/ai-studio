import apiClient from "../utils/apiClient";

// Trusted prompt templates of an app (features/TrustedPromptTemplates.md).
// Listing needs apps:read; proposing, editing and retiring apps:write;
// deleting apps:delete; approving a full administrator.

const base = (appId) => `/apps/${appId}/prompt-templates`;

export const listPromptTemplates = async (appId) => {
  const response = await apiClient.get(base(appId));
  return response.data?.data || [];
};

export const createPromptTemplate = async (appId, template) => {
  const response = await apiClient.post(base(appId), template);
  return response.data?.data;
};

export const updatePromptTemplate = async (appId, id, changes) => {
  const response = await apiClient.patch(`${base(appId)}/${id}`, changes);
  return response.data?.data;
};

export const deletePromptTemplate = async (appId, id) => {
  await apiClient.delete(`${base(appId)}/${id}`);
};

export const approvePromptTemplate = async (appId, id) => {
  const response = await apiClient.post(`${base(appId)}/${id}/approve`);
  return response.data?.data;
};

export const retirePromptTemplate = async (appId, id) => {
  const response = await apiClient.post(`${base(appId)}/${id}/retire`);
  return response.data?.data;
};

/** apiErrorMessage is the API's own error text, else a fallback. */
export const apiErrorMessage = (error, fallback) =>
  error?.response?.data?.errors?.[0]?.detail || error?.message || fallback;
