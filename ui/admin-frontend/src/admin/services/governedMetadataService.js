import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

// Governed Metadata (Enterprise) service module.
// Backend: /api/v1/metadata/* (see api/governed_metadata_handlers.go).

export const METADATA_POINTER_PREFIX = '/data/attributes/governed_metadata/';

const jsonApi = (type, attributes) => ({ data: { type, attributes } });
const flatten = (item) => (item ? { id: item.id, ...(item.attributes || {}) } : null);

/** Returns false (never throws) when the feature is unavailable. */
export const isGovernedMetadataAvailable = async () => {
  try {
    const response = await apiClient.get('/metadata/available');
    return Boolean(response.data?.available);
  } catch (error) {
    return false;
  }
};

export const getMetadataObjectTypes = async () => {
  try {
    const response = await apiClient.get('/metadata/object-types');
    return response.data?.data || [];
  } catch (error) {
    throw handleApiError(error);
  }
};

// ---- Schemas -------------------------------------------------------------

export const getMetadataSchemas = async () => {
  try {
    const response = await apiClient.get('/metadata/schemas');
    return (response.data?.data || []).map(flatten);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getMetadataSchema = async (id) => {
  try {
    const response = await apiClient.get(`/metadata/schemas/${id}`);
    return flatten(response.data?.data);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const createMetadataSchema = async (schema) => {
  try {
    const response = await apiClient.post('/metadata/schemas', jsonApi('MetadataSchema', schema));
    return flatten(response.data?.data);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const updateMetadataSchema = async (id, schema) => {
  try {
    const response = await apiClient.patch(`/metadata/schemas/${id}`, jsonApi('MetadataSchema', schema));
    return flatten(response.data?.data);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const deleteMetadataSchema = async (id) => {
  try {
    await apiClient.delete(`/metadata/schemas/${id}`);
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Resolves the merged schema for an object type.
 * Returns null when the feature is unavailable (403) so callers can hide the UI.
 */
export const resolveMetadataSchema = async (objectType) => {
  try {
    const response = await apiClient.get('/metadata/schemas/resolve', { params: { object_type: objectType } });
    return response.data || null;
  } catch (error) {
    if (error.response?.status === 403) {
      return null;
    }
    throw handleApiError(error);
  }
};

// ---- Vocabularies --------------------------------------------------------

export const getMetadataVocabularies = async () => {
  try {
    const response = await apiClient.get('/metadata/vocabularies');
    return (response.data?.data || []).map(flatten);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getMetadataVocabulary = async (id) => {
  try {
    const response = await apiClient.get(`/metadata/vocabularies/${id}`);
    return flatten(response.data?.data);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const createMetadataVocabulary = async (vocabulary) => {
  try {
    const response = await apiClient.post('/metadata/vocabularies', jsonApi('MetadataVocabulary', vocabulary));
    return flatten(response.data?.data);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const updateMetadataVocabulary = async (id, vocabulary) => {
  try {
    const response = await apiClient.patch(`/metadata/vocabularies/${id}`, jsonApi('MetadataVocabulary', vocabulary));
    return flatten(response.data?.data);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const deleteMetadataVocabulary = async (id) => {
  try {
    await apiClient.delete(`/metadata/vocabularies/${id}`);
  } catch (error) {
    throw handleApiError(error);
  }
};

// ---- Values --------------------------------------------------------------

export const validateObjectMetadata = async (objectType, values) => {
  try {
    const response = await apiClient.post('/metadata/validate', { object_type: objectType, values: values || {} });
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getObjectMetadata = async (objectType, objectId) => {
  try {
    const response = await apiClient.get(`/metadata/objects/${objectType}/${objectId}`);
    return response.data?.data || null;
  } catch (error) {
    if (error.response?.status === 404) {
      return null;
    }
    throw handleApiError(error);
  }
};

export const setObjectMetadata = async (objectType, objectId, values, merge = false) => {
  try {
    const response = await apiClient.put(`/metadata/objects/${objectType}/${objectId}`, { values: values || {}, merge });
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getObjectMetadataAudit = async (objectType, objectId, limit = 50) => {
  try {
    const response = await apiClient.get(`/metadata/objects/${objectType}/${objectId}/audit`, { params: { limit } });
    return response.data?.data || [];
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getMetadataComplianceReport = async (params = {}) => {
  try {
    const response = await apiClient.get('/metadata/compliance', { params });
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/** Users for "user" typed fields. The default /users page size is 10, so ask for all. */
export const getMetadataUsers = async () => {
  try {
    const response = await apiClient.get('/users', { params: { all: true } });
    return response.data?.data || [];
  } catch (error) {
    throw handleApiError(error);
  }
};

// ---- Pure helpers --------------------------------------------------------

/**
 * Maps a 422 response into { [fieldKey]: message }. Errors without a field
 * pointer (e.g. a plugin hook rejection) land under the "_" key.
 * Any other error yields {}.
 */
export const extractGovernedMetadataErrors = (error) => {
  const status = error?.response?.status;
  const errors = error?.response?.data?.errors;
  if (status !== 422 || !Array.isArray(errors)) {
    return {};
  }
  const out = {};
  errors.forEach((e) => {
    const pointer = e?.source?.pointer || '';
    const detail = e?.detail || e?.title || 'Invalid value';
    if (pointer.startsWith(METADATA_POINTER_PREFIX) && pointer.length > METADATA_POINTER_PREFIX.length) {
      const key = pointer.slice(METADATA_POINTER_PREFIX.length);
      out[key] = out[key] ? `${out[key]}; ${detail}` : detail;
    } else {
      out._ = out._ ? `${out._}; ${detail}` : detail;
    }
  });
  return out;
};

/** Converts a ValidationResult into { errors: {field: msg}, warnings: {field: msg} }. */
export const validationResultToFieldMessages = (result) => {
  const collect = (issues) => {
    const out = {};
    (issues || []).forEach((issue) => {
      const key = issue.field || '_';
      out[key] = out[key] ? `${out[key]}; ${issue.message}` : issue.message;
    });
    return out;
  };
  return { errors: collect(result?.errors), warnings: collect(result?.warnings) };
};

/** Drops empty values so cleared fields are not sent as "" / []. */
export const normalizeGovernedMetadataValues = (values) => {
  const out = {};
  Object.entries(values || {}).forEach(([key, value]) => {
    if (value === undefined || value === null) return;
    if (typeof value === 'string' && value.trim() === '') return;
    if (Array.isArray(value) && value.length === 0) return;
    out[key] = value;
  });
  return out;
};
