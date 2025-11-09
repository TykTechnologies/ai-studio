import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

export const createLLM = async (llmData) => {
  try {
    const llmPayload = {
      data: {
        type: "LLM",
        attributes: {
          name: llmData.name,
          api_key: llmData.apiKey,
          api_endpoint: llmData.apiEndpoint,
          privacy_score: llmData.privacyScore,
          short_description: llmData.shortDescription || "",
          long_description: llmData.longDescription || "",
          logo_url: llmData.logoUrl || "",
          vendor: llmData.llmProvider,
          active: llmData.active !== undefined ? llmData.active : true,
          filters: llmData.filters || [],
          default_model: llmData.defaultModel || "",
          allowed_models: llmData.allowedModels || []
        }
      }
    };
    
    const response = await apiClient.post('/llms', llmPayload);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const updateLLM = async (llmId, llmData) => {
  try {
    const llmPayload = {
      data: {
        type: "LLM",
        attributes: {
          name: llmData.name,
          api_key: llmData.apiKey,
          api_endpoint: llmData.apiEndpoint,
          privacy_score: llmData.privacyScore,
          short_description: llmData.shortDescription || "",
          long_description: llmData.longDescription || "",
          logo_url: llmData.logoUrl || "",
          vendor: llmData.llmProvider,
          active: llmData.active !== undefined ? llmData.active : true,
          filters: llmData.filters || [],
          default_model: llmData.defaultModel || "",
          allowed_models: llmData.allowedModels || []
        }
      }
    };
    
    const response = await apiClient.patch(`/llms/${llmId}`, llmPayload);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getAllLLMs = async () => {
  try {
    const response = await apiClient.get('/llms');
    return response.data?.data || [];
  } catch (error) {
    throw handleApiError(error);
  }
};

// LLM Plugin Configuration Methods

export const getLLMPluginConfig = async (llmId, pluginId) => {
  try {
    const response = await apiClient.get(`/llms/${llmId}/plugins/${pluginId}/config`);
    return response.data?.data?.attributes?.config_override || {};
  } catch (error) {
    if (error.response?.status === 404) {
      // If association not found, return empty config
      return {};
    }
    throw handleApiError(error);
  }
};

export const updateLLMPluginConfig = async (llmId, pluginId, configOverride) => {
  try {
    const response = await apiClient.put(`/llms/${llmId}/plugins/${pluginId}/config`, {
      config_override: configOverride,
    });
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};