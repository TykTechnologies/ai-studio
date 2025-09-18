import { useState, useEffect, useCallback } from 'react';
import apiClient from '../utils/apiClient';

const useNamespaces = () => {
  const [namespaces, setNamespaces] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const fetchNamespaces = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    try {
      const response = await apiClient.get('/namespaces');
      if (response.data?.data) {
        const namespaceData = response.data.data.map(ns => ({
          name: ns.attributes.name,
          edgeCount: ns.attributes.edge_count,
          llmCount: ns.attributes.llm_count,
          appCount: ns.attributes.app_count,
          tokenCount: ns.attributes.token_count,
        }));
        setNamespaces(namespaceData);
      }
    } catch (err) {
      console.error('Error fetching namespaces:', err);
      setError(err.response?.data?.message || 'Failed to fetch namespaces');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchNamespaces();
  }, [fetchNamespaces]);

  const getAvailableNamespaces = useCallback(() => {
    return namespaces.filter(ns => ns.edgeCount > 0);
  }, [namespaces]);

  const parseNamespaceString = useCallback((namespaceStr) => {
    if (!namespaceStr || typeof namespaceStr !== 'string') {
      return [];
    }
    return namespaceStr.split(',').map(ns => ns.trim()).filter(ns => ns);
  }, []);

  const formatNamespaceArray = useCallback((namespaceArray) => {
    if (!Array.isArray(namespaceArray)) {
      return '';
    }
    return namespaceArray.filter(ns => ns && typeof ns === 'string').join(', ');
  }, []);

  return {
    namespaces,
    loading,
    error,
    fetchNamespaces,
    getAvailableNamespaces,
    parseNamespaceString,
    formatNamespaceArray,
  };
};

export default useNamespaces;