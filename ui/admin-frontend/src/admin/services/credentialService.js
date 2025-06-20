import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

export const getCredential = async (credentialId) => {
  try {
    const response = await apiClient.get(`/credentials/${credentialId}`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};