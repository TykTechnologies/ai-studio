import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

export const getCatalogues = async (page = 1, all = false) => {
  try {
    const params = all ? { all: true } : { page };
    const response = await apiClient.get('/catalogues', { params });

    if (all) {
      return response.data.data;
    }

    return {
      data: response.data.data,
      totalCount: parseInt(response.headers['x-total-count'] || '0', 10),
      totalPages: parseInt(response.headers['x-total-pages'] || '0', 10)
    };
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getDataCatalogues = async (page = 1, all = false) => {
  try {
    const params = all ? { all: true } : { page };
    const response = await apiClient.get('/data-catalogues', { params });

    if (all) {
      return response.data.data;
    }

    return {
      data: response.data.data,
      totalCount: parseInt(response.headers['x-total-count'] || '0', 10),
      totalPages: parseInt(response.headers['x-total-pages'] || '0', 10)
    };
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getToolCatalogues = async (page = 1, all = false) => {
  try {
    const params = all ? { all: true } : { page };
    const response = await apiClient.get('/tool-catalogues', { params });

    if (all) {
      return response.data;
    }

    return {
      data: response.data,
      totalCount: parseInt(response.headers['x-total-count'] || '0', 10),
      totalPages: parseInt(response.headers['x-total-pages'] || '0', 10)
    };
  } catch (error) {
    throw handleApiError(error);
  }
};