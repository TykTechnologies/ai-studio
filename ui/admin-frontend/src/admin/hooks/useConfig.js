import { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';

const CONFIG_CACHE_KEY = 'tyk_ai_studio_admin_config';
const CACHE_EXPIRY = 60000;

const useConfig = (skipInitialFetch = false) => {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedData = localStorage.getItem(CONFIG_CACHE_KEY);
    if (cachedData) {
      const { data, timestamp } = JSON.parse(cachedData);
      if (Date.now() - timestamp < CACHE_EXPIRY) {
        setConfig(data);
        setLoading(false);
        return data;
      }
    }

    return pubClient.get('/auth/config')
      .then(response => {
        const newData = response.data;
        
        setConfig(newData);
        
        localStorage.setItem(
          CONFIG_CACHE_KEY,
          JSON.stringify({
            data: newData,
            timestamp: Date.now(),
          })
        );
        
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

  return {
    config,
    loading,
    error,
    fetchConfig
  };
};

export default useConfig;