import { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';

const useConfig = (skipInitialFetch = false) => {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedConfig = cacheService.get(CACHE_KEYS.CONFIG);
    if (cachedConfig) {
      setConfig(cachedConfig);
      setLoading(false);
      return cachedConfig;
    }

    return pubClient.get('/auth/config')
      .then(response => {
        const newData = response.data;
        
        setConfig(newData);
        cacheService.set(CACHE_KEYS.CONFIG, newData);
        
        return newData;
      })
      .catch(error => {
        console.error('Failed to fetch config:', error);
        setError(error);
        throw error;
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchConfig();
    }
  }, [fetchConfig, skipInitialFetch]);

  const getDocsLink = useCallback((key) => {
    if (!config || !config.docsLinks) return null;
    
    const link = config.docsLinks[key];
    if (!link) {
      console.error(`Documentation link for key "${key}" not found`);
      return null;
    }
    
    return link;
  }, [config]);

  return {
    config,
    loading,
    error,
    fetchConfig,
    getDocsLink
  };
};

export default useConfig;