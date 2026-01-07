import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

export const createApp = async (appData) => {
  try {
    const appPayload = {
      data: {
        type: "apps",
        attributes: {
          name: appData.name,
          description: appData.description,
          user_id: appData.userId ? parseInt(appData.userId, 10) : null,
          llm_ids: appData.llmIds || [],
          datasource_ids: appData.datasourceIds || [],
          monthly_budget: appData.setBudget ? parseFloat(appData.monthlyBudget) : null,
          budget_start_date: appData.setBudget ? new Date(appData.budgetStartDate).toISOString() : null
        }
      }
    };
    
    const response = await apiClient.post('/apps', appPayload);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const updateApp = async (appId, appData) => {
  try {
    const appPayload = {
      data: {
        type: "apps",
        attributes: {
          name: appData.name,
          description: appData.description,
          user_id: appData.userId ? parseInt(appData.userId, 10) : null,
          llm_ids: appData.llmIds || [],
          datasource_ids: appData.datasourceIds || [],
          monthly_budget: appData.setBudget ? parseFloat(appData.monthlyBudget) : null,
          budget_start_date: appData.setBudget ? new Date(appData.budgetStartDate).toISOString() : null
        }
      }
    };
    
    const response = await apiClient.patch(`/apps/${appId}`, appPayload);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const activateCredential = async (appId) => {
  try {
    const response = await apiClient.post(`/apps/${appId}/activate-credential`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const deactivateCredential = async (appId) => {
  try {
    const response = await apiClient.post(`/apps/${appId}/deactivate-credential`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const resetAppBudget = async (appId) => {
  try {
    const response = await apiClient.post(`/apps/${appId}/reset-budget`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};